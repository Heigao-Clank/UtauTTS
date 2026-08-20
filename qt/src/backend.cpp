#include "backend.h"
#include "utautts_abi.h"

#include <QCoreApplication>
#include <QDateTime>
#include <QDir>
#include <QDrag>
#include <QFile>
#include <QFileInfo>
#include <QMimeData>
#include <QFutureWatcher>
#include <QJsonDocument>
#include <QJsonArray>
#include <QJsonObject>
#include <QNetworkAccessManager>
#include <QNetworkReply>
#include <QNetworkRequest>
#include <QProcess>
#include <QRegularExpression>
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
QString sanitizeLanguageCode(const QString &code) {
    const QString lower = code.trimmed().toLower();
    return lower.isEmpty() ? QStringLiteral("ja") : lower;
}

bool hasResourceLayout(const QDir &root) {
    return root.exists("plugins/renderers") || root.exists("models") || root.exists("voice");
}

QString updateLockPath(const QDir &root) {
    return root.absolutePath() + QStringLiteral(".update-lock.json");
}

bool writePendingUpdateLock(const QDir &root, const QString &version, QString *error) {
    QSaveFile file(updateLockPath(root));
    if (!file.open(QIODevice::WriteOnly)) {
        if (error)
            *error = file.errorString();
        return false;
    }
    const QJsonObject state{
        {QStringLiteral("version"), version},
        {QStringLiteral("started_at"), QDateTime::currentDateTimeUtc().toString(Qt::ISODateWithMs)},
        {QStringLiteral("updater_pid"), 0},
    };
    const QByteArray data = QJsonDocument(state).toJson(QJsonDocument::Compact) + '\n';
    if (file.write(data) != data.size() || !file.commit()) {
        if (error)
            *error = file.errorString();
        file.cancelWriting();
        return false;
    }
    return true;
}

QDir resourceRoot() {
    QDir application(QCoreApplication::applicationDirPath());
    if (application.dirName().compare("app", Qt::CaseInsensitive) == 0) {
        application.cdUp();
    }
    if (hasResourceLayout(application)) {
        return application;
    }

    QDir candidate(QDir::current());
    for (int depth = 0; depth < 8; ++depth) {
        if (hasResourceLayout(candidate)) {
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
      m_language(QSettings().value("appearance/language", QStringLiteral("ja")).toString()),
      m_closeLogOnSuccess(QSettings().value("logging/closeOnSuccess", true).toBool()),
      m_updateCheckEnabled(QSettings().value("appearance/updateCheckEnabled", true).toBool()),
      m_defaultMoraDuration(QSettings().value("synthesis/defaultMoraDuration", 120).toInt()),
      m_defaultPauseDuration(QSettings().value("synthesis/defaultPauseDuration", 180).toInt()),
      m_defaultApplyPitch(QSettings().value("synthesis/defaultApplyPitch", true).toBool()),
      m_synthesizeShortcut(QSettings().value("shortcuts/synthesize", QStringLiteral("Ctrl+Enter")).toString()),
      m_saveProjectShortcut(QSettings().value("shortcuts/saveProject", QStringLiteral("Ctrl+S")).toString()),
      m_reloadVoicebanksShortcut(QSettings().value("shortcuts/reloadVoicebanks", QStringLiteral("Ctrl+O")).toString()),
      m_addUtteranceShortcut(QSettings().value("shortcuts/addUtterance", QStringLiteral("Ctrl+D")).toString()),
      m_removeUtteranceShortcut(QSettings().value("shortcuts/removeUtterance", QStringLiteral("Delete")).toString()),
      m_updateNetwork(new QNetworkAccessManager(this)) {
    m_defaultMoraDuration = qBound(20, m_defaultMoraDuration, 1000);
    m_defaultPauseDuration = qBound(0, m_defaultPauseDuration, 3000);
    const QByteArray dictionaryJSON = QSettings().value("dictionary/entries").toByteArray();
    QJsonParseError parseError;
    const QJsonDocument dictionaryDocument = QJsonDocument::fromJson(dictionaryJSON, &parseError);
    if (parseError.error == QJsonParseError::NoError && dictionaryDocument.isArray()) {
        for (const QJsonValue &value : dictionaryDocument.array()) {
            const QVariantMap entry = value.toObject().toVariantMap();
            const QString surface = entry.value("surface").toString().trimmed();
            const QString reading = entry.value("reading").toString().trimmed();
            if (!surface.isEmpty() && !reading.isEmpty()) {
                m_dictionaryEntries.append(QVariantMap{{"surface", surface}, {"reading", reading}});
            }
        }
    }
}
Backend::~Backend() {
    if (m_updateReply) {
        m_updateReply->abort();
    }
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

void Backend::setLanguage(const QString &value) {
    const QString normalized = sanitizeLanguageCode(value);
    if (m_language == normalized) {
        return;
    }
    m_language = normalized;
    QSettings settings;
    settings.setValue("appearance/language", normalized);
    settings.sync();
    emit languageChanged();
}

QString Backend::loadLanguageFile(const QString &code) const {
    const QString cleaned = sanitizeLanguageCode(code);
    QFile file(QStringLiteral(":/lang/%1.json").arg(cleaned));
    if (!file.open(QIODevice::ReadOnly | QIODevice::Text)) {
        QFile fallback(QStringLiteral(":/lang/ja.json"));
        if (!fallback.open(QIODevice::ReadOnly | QIODevice::Text))
            return QString();
        return QString::fromUtf8(fallback.readAll());
    }
    return QString::fromUtf8(file.readAll());
}

QStringList Backend::languageCodes() const {
    const QStringList files = QDir(QStringLiteral(":/lang"))
            .entryList({QStringLiteral("*.json")}, QDir::Files, QDir::Name);
    QStringList codes;
    for (const QString &file : files) {
        if (file.compare(QLatin1String("lang.json"), Qt::CaseInsensitive) == 0)
            continue;
        codes.append(file.left(file.size() - QStringLiteral(".json").size()));
    }
    if (codes.isEmpty()) {
        codes = languageDisplayNames().keys();
        codes.sort();
    }
    return codes;
}

QHash<QString, QString> Backend::languageDisplayNames() const {
    if (!m_languageNamesLoaded) {
        m_languageNamesLoaded = true;
        QFile file(QStringLiteral(":/lang/lang.json"));
        if (file.open(QIODevice::ReadOnly | QIODevice::Text)) {
            const QJsonDocument document = QJsonDocument::fromJson(file.readAll());
            if (document.isObject()) {
                const QJsonObject object = document.object();
                for (auto it = object.begin(); it != object.end(); ++it)
                    m_languageNames.insert(it.key(), it.value().toString());
            }
        }
    }
    return m_languageNames;
}

QString Backend::languageDisplayName(const QString &code) const {
    const QString cleaned = sanitizeLanguageCode(code);
    return languageDisplayNames().value(cleaned, cleaned);
}

QString Backend::suppressedUpdateVersion() const {
    return QSettings().value(QStringLiteral("appearance/suppressedUpdateVersion"), QString()).toString();
}

void Backend::setSuppressedUpdateVersion(const QString &version) {
    QSettings settings;
    settings.setValue(QStringLiteral("appearance/suppressedUpdateVersion"), version);
    settings.sync();
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

void Backend::setUpdateCheckEnabled(bool value) {
    if (m_updateCheckEnabled == value) {
        return;
    }
    m_updateCheckEnabled = value;
    QSettings settings;
    settings.setValue("appearance/updateCheckEnabled", value);
    settings.sync();
    emit updateSettingsChanged();
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

void Backend::setShortcutSequences(const QString &synthesize,
                                   const QString &saveProject,
                                   const QString &reloadVoicebanks,
                                   const QString &addUtterance,
                                   const QString &removeUtterance) {
    if (m_synthesizeShortcut == synthesize.trimmed()
            && m_saveProjectShortcut == saveProject.trimmed()
            && m_reloadVoicebanksShortcut == reloadVoicebanks.trimmed()
            && m_addUtteranceShortcut == addUtterance.trimmed()
            && m_removeUtteranceShortcut == removeUtterance.trimmed()) {
        return;
    }
    m_synthesizeShortcut = synthesize.trimmed();
    m_saveProjectShortcut = saveProject.trimmed();
    m_reloadVoicebanksShortcut = reloadVoicebanks.trimmed();
    m_addUtteranceShortcut = addUtterance.trimmed();
    m_removeUtteranceShortcut = removeUtterance.trimmed();
    QSettings settings;
    settings.setValue("shortcuts/synthesize", m_synthesizeShortcut);
    settings.setValue("shortcuts/saveProject", m_saveProjectShortcut);
    settings.setValue("shortcuts/reloadVoicebanks", m_reloadVoicebanksShortcut);
    settings.setValue("shortcuts/addUtterance", m_addUtteranceShortcut);
    settings.setValue("shortcuts/removeUtterance", m_removeUtteranceShortcut);
    settings.sync();
    emit shortcutSettingsChanged();
}

void Backend::setDictionaryEntries(const QVariantList &entries) {
    QVariantList normalized;
    for (const QVariant &value : entries) {
        const QVariantMap entry = value.toMap();
        const QString surface = entry.value("surface").toString().trimmed();
        const QString reading = entry.value("reading").toString().trimmed();
        if (!surface.isEmpty() && !reading.isEmpty()) {
            normalized.append(QVariantMap{{"surface", surface}, {"reading", reading}});
        }
    }
    if (m_dictionaryEntries == normalized) {
        return;
    }
    m_dictionaryEntries = normalized;
    QSettings settings;
    settings.setValue("dictionary/entries", QJsonDocument(QJsonArray::fromVariantList(m_dictionaryEntries)).toJson(QJsonDocument::Compact));
    settings.sync();
    emit dictionaryChanged();
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
    const QString text = QStringLiteral("UtauTTS %1 \n\nDeveloped by yh（@2237yh）\nTesting by アアアアアアア（@a7_riri）\n\nUTAUボイスバンクの原音接続に、学習ベースのイントネーション調整を加えた日本語TTS").arg(QCoreApplication::applicationVersion());
    MessageBoxW(GetActiveWindow(),
                reinterpret_cast<LPCWSTR>(text.utf16()),
                reinterpret_cast<LPCWSTR>(title.utf16()),
                MB_OK | MB_ICONINFORMATION);
    return true;
#else
    return false;
#endif
}

bool Backend::startUpdateDownload(const QString &downloadUrl, const QString &version) {
    if (downloadUrl.isEmpty()) {
        const QString message = tr("ダウンロードURLを取得できませんでした。");
        setError(message);
        showUpdateError(tr("更新に失敗しました"), message);
        return false;
    }
    QString safeVersion = version.trimmed();
    safeVersion.replace(QRegularExpression(QStringLiteral("[^A-Za-z0-9._-]")), QStringLiteral("_"));
    const QString fileName = safeVersion.isEmpty()
        ? QStringLiteral("utautts-update.zip")
        : QStringLiteral("utautts-update-%1.zip").arg(safeVersion);
    const QString zipPath = QDir(QDir::tempPath()).filePath(fileName);

    if (m_updateReply) {
        m_updateReply->abort();
        m_updateReply->deleteLater();
        m_updateReply = nullptr;
    }
    delete m_updateFile;
    m_updateFile = nullptr;
    m_updateCancelled = false;
    m_updateWriteError.clear();
    QFile::remove(zipPath);
    m_updateFile = new QFile(zipPath, this);
    if (!m_updateFile->open(QIODevice::WriteOnly | QIODevice::Truncate)) {
        delete m_updateFile;
        m_updateFile = nullptr;
        const QString message = tr("ダウンロード先の一時ファイルを開けませんでした。");
        setError(message);
        showUpdateError(tr("更新に失敗しました"), message);
        return false;
    }

    QNetworkRequest request{QUrl(downloadUrl)};
    request.setTransferTimeout(10 * 60 * 1000);
    QNetworkReply *reply = m_updateNetwork->get(request);
    m_updateReply = reply;
    connect(reply, &QNetworkReply::readyRead, this, [this, reply] {
        if (m_updateReply == reply && m_updateFile) {
            const QByteArray data = reply->readAll();
            if (!data.isEmpty() && m_updateFile->write(data) != data.size()) {
                m_updateWriteError = m_updateFile->errorString();
                if (m_updateWriteError.isEmpty())
                    m_updateWriteError = tr("更新ファイルを一時ディレクトリへ書き込めませんでした。");
                reply->abort();
            }
        }
    });
    connect(reply, &QNetworkReply::downloadProgress, this, [this, reply](qint64 received, qint64 total) {
        if (m_updateReply == reply) {
            emit updateDownloadProgress(received, total);
        }
    });
    connect(reply, &QNetworkReply::finished, this, [this, reply] {
        if (m_updateReply != reply || !m_updateFile) {
            return;
        }
        const QNetworkReply::NetworkError networkError = reply->error();
        const bool cancelled = m_updateCancelled && networkError == QNetworkReply::OperationCanceledError;
        const QString errorText = reply->errorString();
        const QString path = m_updateFile->fileName();
        if (!m_updateFile->flush() && m_updateWriteError.isEmpty())
            m_updateWriteError = m_updateFile->errorString();
        const QString writeError = m_updateWriteError;
        delete m_updateFile;
        m_updateFile = nullptr;
        m_updateReply = nullptr;
        m_updateCancelled = false;
        m_updateWriteError.clear();
        reply->deleteLater();
        if (cancelled) {
            QFile::remove(path);
            return;
        }
        if (networkError == QNetworkReply::NoError && writeError.isEmpty()) {
            emit updateDownloadFinished(true, path);
            return;
        }
        QFile::remove(path);
        const QString detail = !writeError.isEmpty()
            ? writeError
            : networkError == QNetworkReply::OperationCanceledError
            ? tr("タイムアウトしました。通信環境を確認して、再度お試しください。")
            : errorText;
        setError(tr("更新ファイルのダウンロードに失敗しました: %1").arg(detail));
        showUpdateError(tr("更新に失敗しました"), tr("更新ファイルのダウンロードに失敗しました。\n%1\n\nリリースページから手動で更新するか、時間をおいて再度お試しください。").arg(detail));
        emit updateDownloadFinished(false, QString());
    });
    return true;
}

bool Backend::installUpdate(const QString &localZip, const QString &version) {
    const QDir root = resourceRoot();
    const QString updaterPath = root.filePath(QStringLiteral("tools/utautts-updater.exe"));
    if (!QFileInfo::exists(updaterPath)) {
        const QString message = tr("アップデータが同梱されていません。リリースページから手動で更新してください。");
        setError(message);
        showUpdateError(tr("更新に失敗しました"), message);
        return false;
    }
    if (localZip.isEmpty() || !QFileInfo::exists(localZip)) {
        const QString message = tr("ダウンロードした更新ファイルが見つかりません。");
        setError(message);
        showUpdateError(tr("更新に失敗しました"), message);
        return false;
    }
    const QString tempUpdater = QDir(QDir::tempPath()).filePath(
        QStringLiteral("utautts-updater-%1.exe").arg(QCoreApplication::applicationPid()));
    QFile::remove(tempUpdater);
    if (!QFile::copy(updaterPath, tempUpdater)) {
        const QString message = tr("アップデータを一時ディレクトリに配置できませんでした。");
        setError(message);
        showUpdateError(tr("更新に失敗しました"), message);
        return false;
    }
    const QStringList arguments{
        QStringLiteral("-target"), QDir::toNativeSeparators(root.absolutePath()),
        QStringLiteral("-zip"), QDir::toNativeSeparators(localZip),
        QStringLiteral("-delete-zip"),
        QStringLiteral("-pid"), QString::number(QCoreApplication::applicationPid()),
        QStringLiteral("-version"), version,
    };
    QString lockError;
    if (!writePendingUpdateLock(root, version, &lockError)) {
        const QString message = tr("更新中ロックを作成できませんでした: %1").arg(lockError);
        setError(message);
        showUpdateError(tr("更新に失敗しました"), message);
        return false;
    }
    qint64 updaterPid = 0;
    if (!QProcess::startDetached(tempUpdater, arguments, QDir::tempPath(), &updaterPid)) {
        QFile::remove(updateLockPath(root));
        const QString message = tr("アップデータを起動できませんでした。");
        setError(message);
        showUpdateError(tr("更新に失敗しました"), message);
        return false;
    }
    setError({});
    return true;
}

void Backend::cancelUpdateDownload() {
    m_updateCancelled = true;
    if (m_updateReply) {
        m_updateReply->abort();
    }
}

void Backend::showUpdateError(const QString &title, const QString &text) {
#ifdef Q_OS_WIN
    MessageBoxW(GetActiveWindow(),
                reinterpret_cast<LPCWSTR>(text.utf16()),
                reinterpret_cast<LPCWSTR>(title.utf16()),
                MB_OK | MB_ICONWARNING);
#endif
}

void Backend::initialize() {
    const QDir root = resourceRoot();
    QJsonObject config{{"voice_dir", root.filePath("voice")}};
    config.insert("renderer_directories", QJsonArray{root.filePath("plugins/renderers")});
    config.insert("model_directories", QJsonArray{root.filePath("models")});
    const QString runtime = root.filePath("runtime");
    const QString openJTalkPath = QDir(runtime).filePath("utautts-openjtalk-features.exe");
    const QString openJTalkDictionary = QDir(runtime).filePath("open_jtalk_dic_utf_8-1.11");
    if (QFileInfo(openJTalkPath).isFile()) {
        config.insert("openjtalk_path", openJTalkPath);
    }
    if (QFileInfo(openJTalkDictionary).isDir()) {
        config.insert("openjtalk_dictionary", openJTalkDictionary);
    }
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
    const QJsonValue result = object.value("result");
    if (!result.isObject()) {
        throw std::runtime_error("native backend returned no result");
    }
    return result.toObject().toVariantMap();
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
    if (m_busy) {
        return;
    }
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
    const QVariantList dictionary = m_dictionaryEntries;
    const auto future = QtConcurrent::run([this, text, dictionary]() {
        try {
            return call("analyze", {{"text", text}, {"dictionary", dictionary}});
        } catch (const std::exception &exception) {
            return QVariantMap{{"_error", QString::fromUtf8(exception.what())}};
        }
    });
    ++m_activeCallCount;
    m_activeCalls.addFuture(future);
    watcher->setFuture(future);
}

void Backend::predictProsody(const QVariantMap &request) {
    if (m_busy) {
        return;
    }
    QString requestId = request.value("request_id").toString();
    if (requestId.isEmpty()) {
        requestId = QUuid::createUuid().toString(QUuid::WithoutBraces);
    }
    const quint64 generation = ++m_nextProsodyGeneration;
    auto *watcher = new QFutureWatcher<QVariantMap>(this);
    connect(watcher, &QFutureWatcher<QVariantMap>::finished, this,
            [this, watcher, generation, requestId]() {
                const QVariantMap value = watcher->result();
                if (generation == m_nextProsodyGeneration) {
                    if (value.contains("_error")) {
                        setError(value.value("_error").toString());
                    } else {
                        m_prosodyRequestId = requestId;
                        m_prosodyJson = QString::fromUtf8(
                            QJsonDocument::fromVariant(value).toJson(QJsonDocument::Compact));
                        emit prosodyChanged();
                        setError({});
                    }
                }
                watcher->deleteLater();
                if (--m_activeCallCount == 0) {
                    m_activeCalls.clearFutures();
                }
            });
    QVariantMap callRequest = request;
    callRequest.insert("request_id", requestId);
    const auto future = QtConcurrent::run([this, callRequest]() {
        try {
            return call("predictProsody", callRequest);
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
                    m_synthesisJson = QString::fromUtf8(
                        QJsonDocument::fromVariant(result).toJson(QJsonDocument::Compact));
                    emit synthesisChanged();
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

bool Backend::startFileDrag(const QVariantList &files) {
    QList<QUrl> urls;
    urls.reserve(files.size());
    for (const QVariant &value : files) {
        const QUrl url = value.canConvert<QUrl>() ? value.toUrl() : QUrl(value.toString());
        if (!url.isLocalFile() || url.toLocalFile().isEmpty() || !QFileInfo::exists(url.toLocalFile())) {
            setError(tr("ドラッグするWAVファイルが見つかりません"));
            return false;
        }
        urls.append(QUrl::fromLocalFile(QFileInfo(url.toLocalFile()).absoluteFilePath()));
    }
    if (urls.isEmpty()) {
        setError(tr("ドラッグするWAVファイルがありません"));
        return false;
    }

    auto *mimeData = new QMimeData;
    mimeData->setUrls(urls);
    auto *drag = new QDrag(this);
    drag->setMimeData(mimeData);
    drag->exec(Qt::CopyAction);
    setError({});
    return true;
}

QUrl Backend::writeDragExo(const QUrl &directory, const QVariantList &files, int frameRate) {
    if (!directory.isLocalFile() || files.isEmpty()) {
        setError(tr("ドラッグ用のexoファイルを作成できません"));
        return {};
    }
    QStringList paths;
    paths.reserve(files.size());
    for (const QVariant &value : files) {
        const QUrl url = value.canConvert<QUrl>() ? value.toUrl() : QUrl(value.toString());
        if (!url.isLocalFile() || url.toLocalFile().isEmpty() || !QFileInfo::exists(url.toLocalFile())) {
            setError(tr("ドラッグするWAVファイルが見つかりません"));
            return {};
        }
        paths.append(QFileInfo(url.toLocalFile()).absoluteFilePath());
    }
    const int boundedFrameRate = qBound(1, frameRate, 240);
    const QString exoPath = QDir(directory.toLocalFile()).filePath(QStringLiteral("utautts.exo"));
    const QVariantMap request{{"output_path", QDir::toNativeSeparators(exoPath)}, {"files", paths}, {"frame_rate", boundedFrameRate}};
    try {
        call("writeExo", request);
    } catch (const std::exception &exception) {
        setError(QString::fromUtf8(exception.what()));
        return {};
    }
    setError({});
    return QUrl::fromLocalFile(exoPath);
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

bool Backend::saveProject(const QUrl &destination, const QVariantMap &project) {
    if (!destination.isLocalFile()) {
        setError(tr("プロジェクトの保存先が無効です"));
        return false;
    }
    const QJsonDocument document = QJsonDocument::fromVariant(project);
    if (!document.isObject()) {
        setError(tr("プロジェクトのデータが無効です"));
        return false;
    }
    const QByteArray data = document.toJson(QJsonDocument::Indented);
    QSaveFile target(destination.toLocalFile());
    if (!target.open(QIODevice::WriteOnly) || target.write(data) != data.size() || !target.commit()) {
        target.cancelWriting();
        setError(tr("プロジェクトを保存できませんでした"));
        return false;
    }
    setError({});
    return true;
}

QVariantMap Backend::loadProject(const QUrl &source) {
    if (!source.isLocalFile()) {
        setError(tr("プロジェクトファイルが無効です"));
        return {{"_error", error()}};
    }
    QFile file(source.toLocalFile());
    if (!file.open(QIODevice::ReadOnly)) {
        setError(tr("プロジェクトファイルを開けませんでした"));
        return {{"_error", error()}};
    }
    QJsonParseError parseError;
    const QJsonDocument document = QJsonDocument::fromJson(file.readAll(), &parseError);
    if (parseError.error != QJsonParseError::NoError || !document.isObject()) {
        setError(tr("プロジェクトファイルの形式が正しくありません"));
        return {{"_error", error()}};
    }
    const QVariantMap project = document.toVariant().toMap();
    const QVariantList utterances = project.value("utterances").toList();
    if (project.value("format").toString() != "utautts-project"
            || project.value("format_version").toInt() < 1 || !project.contains("utterances")) {
        setError(tr("対応していないプロジェクト形式です"));
        return {{"_error", error()}};
    }
    Q_UNUSED(utterances)
    setError({});
    return project;
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
