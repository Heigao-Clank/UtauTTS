#include "backend.h"
#include "utautts_abi.h"

#include <QCoreApplication>
#include <QDateTime>
#include <QDir>
#include <QFile>
#include <QFileInfo>
#include <QFutureWatcher>
#include <QJsonDocument>
#include <QJsonArray>
#include <QJsonObject>
#include <QSaveFile>
#include <QStandardPaths>
#include <QSettings>
#include <QUuid>
#include <QtConcurrent>
#include <memory>
#include <stdexcept>

#ifdef Q_OS_WIN
#include <windows.h>
#endif

namespace {
QDir resourceRoot() {
    QDir application(QCoreApplication::applicationDirPath());
    if (application.dirName().compare("app", Qt::CaseInsensitive) == 0) {
        application.cdUp();
        return application;
    }

    QDir candidate(QDir::current());
    for (int depth = 0; depth < 8; ++depth) {
        if (candidate.exists("plugins/renderers") || candidate.exists("models") || candidate.exists("voice")) {
            return candidate;
        }
        if (!candidate.cdUp()) {
            break;
        }
    }
    return application;
}
}

Backend::Backend(QObject *parent)
    : QObject(parent),
      m_darkMode(QSettings().value("appearance/darkMode", false).toBool()),
      m_closeLogOnSuccess(QSettings().value("logging/closeOnSuccess", true).toBool()),
      m_defaultMoraDuration(QSettings().value("synthesis/defaultMoraDuration", 120).toInt()),
      m_defaultPauseDuration(QSettings().value("synthesis/defaultPauseDuration", 180).toInt()),
      m_defaultApplyPitch(QSettings().value("synthesis/defaultApplyPitch", true).toBool()) {
    m_defaultMoraDuration = qBound(20, m_defaultMoraDuration, 1000);
    m_defaultPauseDuration = qBound(0, m_defaultPauseDuration, 3000);
}
Backend::~Backend() {
    m_activeCalls.waitForFinished();
    if (m_handle) {
        UtauTTSDestroy(m_handle);
    }
}

void Backend::setDarkMode(bool value) {
    if (m_darkMode == value) {
        return;
    }
    m_darkMode = value;
    QSettings settings;
    settings.setValue("appearance/darkMode", value);
    settings.sync();
    emit themeChanged();
}

void Backend::setCloseLogOnSuccess(bool value) {
    if (m_closeLogOnSuccess == value) {
        return;
    }
    m_closeLogOnSuccess = value;
    QSettings settings;
    settings.setValue("logging/closeOnSuccess", value);
    settings.sync();
    emit logSettingsChanged();
}

void Backend::setSynthesisDefaults(int moraDuration, int pauseDuration, bool applyPitch) {
    const int boundedMoraDuration = qBound(20, moraDuration, 1000);
    const int boundedPauseDuration = qBound(0, pauseDuration, 3000);
    if (m_defaultMoraDuration == boundedMoraDuration
            && m_defaultPauseDuration == boundedPauseDuration
            && m_defaultApplyPitch == applyPitch) {
        return;
    }
    m_defaultMoraDuration = boundedMoraDuration;
    m_defaultPauseDuration = boundedPauseDuration;
    m_defaultApplyPitch = applyPitch;
    QSettings settings;
    settings.setValue("synthesis/defaultMoraDuration", m_defaultMoraDuration);
    settings.setValue("synthesis/defaultPauseDuration", m_defaultPauseDuration);
    settings.setValue("synthesis/defaultApplyPitch", m_defaultApplyPitch);
    settings.sync();
    emit synthesisDefaultsChanged();
}

void Backend::appendLog(const QString &message) {
    if (message.trimmed().isEmpty()) {
        return;
    }
    const QString timestamp = QDateTime::currentDateTime().toString("HH:mm:ss");
    m_logLines.append(QStringLiteral("[%1] %2").arg(timestamp, message));
    constexpr int maxLogLines = 500;
    while (m_logLines.size() > maxLogLines) {
        m_logLines.removeFirst();
    }
    emit logsChanged();
}

void Backend::clearLogs() {
    if (m_logLines.isEmpty()) {
        return;
    }
    m_logLines.clear();
    emit logsChanged();
}

bool Backend::showNativeAboutDialog() {
#ifdef Q_OS_WIN
    const QString title = tr("UtauTTSについて");
    const QString text = QStringLiteral("UtauTTS %1\n\nUTAU音源を利用する音声合成ソフトウェア")
                             .arg(QCoreApplication::applicationVersion());
    MessageBoxW(GetActiveWindow(),
                reinterpret_cast<LPCWSTR>(text.utf16()),
                reinterpret_cast<LPCWSTR>(title.utf16()),
                MB_OK | MB_ICONINFORMATION);
    return true;
#else
    return false;
#endif
}

void Backend::initialize() {
    const QDir root = resourceRoot();
    QJsonObject config{{"voice_dir", root.filePath("voice")}};
    config.insert("renderer_directories", QJsonArray{root.filePath("plugins/renderers")});
    config.insert("model_directories", QJsonArray{root.filePath("models")});
    const QString runtime = root.filePath("runtime");
    config.insert("openjtalk_path", QDir(runtime).filePath("utautts-openjtalk-features.exe"));
    config.insert("openjtalk_dictionary", QDir(runtime).filePath("open_jtalk_dic_utf_8-1.11"));
    QByteArray encoded = QJsonDocument(config).toJson(QJsonDocument::Compact);
    m_handle = UtauTTSCreate(encoded.data());
    if (!m_handle) {
        std::unique_ptr<char, decltype(&UtauTTSFree)> detail(UtauTTSLastError(), &UtauTTSFree);
        const QString message = detail ? QString::fromUtf8(detail.get()) : QString();
        setError(message.isEmpty() ? tr("Goバックエンドを初期化できませんでした") : message);
        return;
    }
    emit connectedChanged();
    try { refreshMetadata(); setError({}); } catch (const std::exception &exception) { setError(QString::fromUtf8(exception.what())); }
}

QVariantMap Backend::call(const QByteArray &method, const QVariantMap &request) {
    if (!m_handle) {
        throw std::runtime_error("backend is not initialized");
    }
    QByteArray methodCopy = method;
    QByteArray requestJSON = QJsonDocument::fromVariant(request).toJson(QJsonDocument::Compact);
    std::unique_ptr<char, decltype(&UtauTTSFree)> response(
        UtauTTSCall(m_handle, methodCopy.data(), requestJSON.data()), &UtauTTSFree);
    if (!response) {
        throw std::runtime_error("native backend returned no response");
    }
    QJsonParseError parseError;
    const QJsonDocument document = QJsonDocument::fromJson(response.get(), &parseError);
    if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
        throw std::runtime_error("native backend returned invalid JSON");
    }
    const QJsonObject object = document.object();
    if (!object.value("ok").toBool()) {
        throw std::runtime_error(object.value("error").toString().toStdString());
    }
    return object.value("result").toObject().toVariantMap();
}

void Backend::refreshMetadata() {
    const QVariantMap voices = call("voicebanks");
    const QVariantMap models = call("models");
    const QVariantMap renderers = call("renderers");
    const QVariantMap hardware = call("hardware");
    m_voicebanks = voices.value("voicebanks").toList();
    m_models = models.value("models").toList();
    m_renderers = renderers.value("renderers").toList();
    m_defaultRenderer = renderers.value("default_renderer").toString();
    m_cudaAvailable = hardware.value("cuda_available").toBool();
    emit metadataChanged();
}

void Backend::reloadVoicebanks() {
    if (m_busy) {
        return;
    }
    setBusy(true);
    setError({});
    auto *watcher = new QFutureWatcher<QVariantMap>(this);
    connect(watcher, &QFutureWatcher<QVariantMap>::finished, this, [this, watcher]() {
        setBusy(false);
        const QVariantMap result = watcher->result();
        if (result.contains("_error")) {
            setError(result.value("_error").toString());
        } else {
            m_voicebanks = result.value("voicebanks").toList();
            emit metadataChanged();
        }
        watcher->deleteLater();
        if (--m_activeCallCount == 0) {
            m_activeCalls.clearFutures();
        }
    });
    const auto future = QtConcurrent::run([this]() {
        try {
            return call("reloadVoicebanks");
        } catch (const std::exception &exception) {
            return QVariantMap{{"_error", QString::fromUtf8(exception.what())}};
        }
    });
    ++m_activeCallCount;
    m_activeCalls.addFuture(future);
    watcher->setFuture(future);
}

void Backend::analyze(const QString &text, const QString &requestId) {
    const quint64 generation = ++m_nextAnalysisGeneration;
    m_analysisGenerations.insert(requestId, generation);
    auto *watcher = new QFutureWatcher<QVariantMap>(this);
    connect(watcher, &QFutureWatcher<QVariantMap>::finished, this,
            [this, watcher, generation, requestId, text]() {
                const QVariantMap value = watcher->result();
                if (m_analysisGenerations.value(requestId) == generation) {
                    m_analysisGenerations.remove(requestId);
                    if (value.contains("_error")) {
                        setError(value.value("_error").toString());
                    } else {
                        m_analysisRequestId = requestId;
                        m_analysisSourceText = text;
                        m_analysisJson = QString::fromUtf8(
                            QJsonDocument::fromVariant(value).toJson(QJsonDocument::Compact));
                        emit analysisChanged();
                        setError({});
                    }
                }
                watcher->deleteLater();
                if (--m_activeCallCount == 0) {
                    m_activeCalls.clearFutures();
                }
            });
    const auto future = QtConcurrent::run([this, text]() {
        try {
            return call("analyze", {{"text", text}});
        } catch (const std::exception &exception) {
            return QVariantMap{{"_error", QString::fromUtf8(exception.what())}};
        }
    });
    ++m_activeCallCount;
    m_activeCalls.addFuture(future);
    watcher->setFuture(future);
}

void Backend::synthesize(const QVariantMap &input) {
    if (m_busy) {
        return;
    }
    if (!m_previewDirectory.isValid()) {
        setError(tr("プレビュー用の一時ディレクトリを作成できませんでした"));
        return;
    }
    QVariantMap request = input;
    const QString outputPath = m_previewDirectory.filePath(
        "utautts-" + QUuid::createUuid().toString(QUuid::WithoutBraces) + ".wav");
    request.insert("output_path", outputPath);
    appendLog(tr("音声合成を開始しました: %1").arg(request.value("text").toString()));
    setBusy(true);
    setError({});
    auto *watcher = new QFutureWatcher<QVariantMap>(this);
    connect(watcher, &QFutureWatcher<QVariantMap>::finished, this,
            [this, watcher, outputPath]() {
                setBusy(false);
                const QVariantMap result = watcher->result();
                if (result.contains("_error")) {
                    const QString error = result.value("_error").toString();
                    appendLog(tr("音声合成に失敗しました: %1").arg(error));
                    setError(error);
                } else {
                    m_previewPath = outputPath;
                    m_previewUrl = QUrl::fromLocalFile(outputPath);
                    appendLog(tr("音声合成が完了しました。"));
                    emit previewReady();
                }
                watcher->deleteLater();
                if (--m_activeCallCount == 0) {
                    m_activeCalls.clearFutures();
                }
            });
    const auto future = QtConcurrent::run([this, request]() {
        try {
            return call("synthesize", request);
        } catch (const std::exception &exception) {
            return QVariantMap{{"_error", QString::fromUtf8(exception.what())}};
        }
    });
    ++m_activeCallCount;
    m_activeCalls.addFuture(future);
    watcher->setFuture(future);
}

bool Backend::savePreview(const QUrl &destination) {
    if (m_previewPath.isEmpty() || !destination.isLocalFile()) {
        setError(tr("保存できるプレビュー音声がありません"));
        return false;
    }
    QFile source(m_previewPath);
    QSaveFile target(destination.toLocalFile());
    if (!source.open(QIODevice::ReadOnly) || !target.open(QIODevice::WriteOnly)) {
        setError(tr("WAVの保存先を開けませんでした"));
        return false;
    }
    constexpr qint64 chunkSize = 1024 * 1024;
    while (!source.atEnd()) {
        const QByteArray chunk = source.read(chunkSize);
        if (chunk.isEmpty() && source.error() != QFileDevice::NoError) {
            target.cancelWriting();
            setError(tr("プレビューWAVを読み込めませんでした"));
            return false;
        }
        if (target.write(chunk) != chunk.size()) {
            target.cancelWriting();
            setError(tr("WAVを保存できませんでした"));
            return false;
        }
    }
    if (!target.commit()) {
        setError(tr("WAVを保存できませんでした"));
        return false;
    }
    setError({});
    return true;
}

QUrl Backend::defaultSaveFile(const QString &fileName) const {
    const QFileInfo fileInfo(fileName);
    if (fileName.isEmpty() || fileInfo.fileName() != fileName) {
        return {};
    }
    QString directoryPath = QStandardPaths::writableLocation(QStandardPaths::DocumentsLocation);
    if (directoryPath.isEmpty()) {
        directoryPath = QDir::homePath();
    }
    return QUrl::fromLocalFile(QDir(directoryPath).filePath(fileName));
}

QUrl Backend::fileInDirectory(const QUrl &directory, const QString &fileName) const {
    const QFileInfo fileInfo(fileName);
    if (!directory.isLocalFile() || fileName.isEmpty() || fileInfo.fileName() != fileName) {
        return {};
    }
    return QUrl::fromLocalFile(QDir(directory.toLocalFile()).filePath(fileName));
}

void Backend::setBusy(bool value) {
    if (m_busy == value) {
        return;
    }
    m_busy = value;
    emit busyChanged();
}

void Backend::setError(const QString &value) {
    if (m_error == value) {
        return;
    }
    m_error = value;
    emit errorChanged();
}
