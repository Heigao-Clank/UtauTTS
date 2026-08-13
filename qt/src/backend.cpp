#include "backend.h"
#include "utautts_abi.h"

#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QFutureWatcher>
#include <QJsonDocument>
#include <QJsonArray>
#include <QJsonObject>
#include <QSaveFile>
#include <QUuid>
#include <QtConcurrent>
#include <memory>
#include <stdexcept>

Backend::Backend(QObject *parent) : QObject(parent) {}
Backend::~Backend() {
    // Every worker captures this object in order to call the native handle.
    // Keep both alive until those workers have stopped during application exit.
    m_activeCalls.waitForFinished();
    if (m_handle) {
        UtauTTSDestroy(m_handle);
    }
}

void Backend::initialize() {
    QDir root(QCoreApplication::applicationDirPath());
    if (!root.exists("voice") && root.cdUp() && !root.exists("voice")) {
        root = QDir(QCoreApplication::applicationDirPath());
    }
    QJsonObject config{{"voice_dir", root.filePath("voice")}};
    config.insert("renderer_directories", QJsonArray{root.filePath("plugins/renderers")});
    config.insert("model_directories", QJsonArray{root.filePath("models")});
    const QString runtime = root.filePath("runtime");
    config.insert("openjtalk_path", QDir(runtime).filePath("utautts-openjtalk-features.exe"));
    config.insert("openjtalk_dictionary", QDir(runtime).filePath("open_jtalk_dic_utf_8-1.11"));
    QByteArray encoded = QJsonDocument(config).toJson(QJsonDocument::Compact);
    m_handle = UtauTTSCreate(encoded.data());
    if (!m_handle) { setError(tr("Goバックエンドを初期化できませんでした")); return; }
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
    m_voicebanks = voices.value("voicebanks").toList();
    m_models = models.value("models").toList();
    m_renderers = renderers.value("renderers").toList();
    m_defaultRenderer = renderers.value("default_renderer").toString();
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
    const quint64 generation = ++m_analysisGeneration;
    auto *watcher = new QFutureWatcher<QVariantMap>(this);
    connect(watcher, &QFutureWatcher<QVariantMap>::finished, this,
            [this, watcher, generation, requestId, text]() {
                const QVariantMap value = watcher->result();
                if (generation == m_analysisGeneration) {
                    if (value.contains("_error")) {
                        setError(value.value("_error").toString());
                    } else {
                        emit analysisReady(requestId, text, value);
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
    setBusy(true);
    setError({});
    auto *watcher = new QFutureWatcher<QVariantMap>(this);
    connect(watcher, &QFutureWatcher<QVariantMap>::finished, this,
            [this, watcher, outputPath]() {
                setBusy(false);
                const QVariantMap result = watcher->result();
                if (result.contains("_error")) {
                    setError(result.value("_error").toString());
                } else {
                    m_previewPath = outputPath;
                    emit synthesisReady(QUrl::fromLocalFile(outputPath), result);
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
