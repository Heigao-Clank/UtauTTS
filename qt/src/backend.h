#pragma once

#include <QObject>
#include <QFutureSynchronizer>
#include <QHash>
#include <QTemporaryDir>
#include <QUrl>
#include <QStringList>
#include <QVariantList>
#include <QVariantMap>
#include <cstdint>

class Backend final : public QObject {
    Q_OBJECT
    Q_PROPERTY(bool connected READ connected NOTIFY connectedChanged)
    Q_PROPERTY(bool busy READ busy NOTIFY busyChanged)
    Q_PROPERTY(QString error READ error NOTIFY errorChanged)
    Q_PROPERTY(QVariantList voicebanks READ voicebanks NOTIFY metadataChanged)
    Q_PROPERTY(QVariantList models READ models NOTIFY metadataChanged)
    Q_PROPERTY(QVariantList renderers READ renderers NOTIFY metadataChanged)
    Q_PROPERTY(QVariantList dictionaryEntries READ dictionaryEntries NOTIFY dictionaryChanged)
    Q_PROPERTY(QString defaultRenderer READ defaultRenderer NOTIFY metadataChanged)
    Q_PROPERTY(bool cudaAvailable READ cudaAvailable NOTIFY metadataChanged)
    Q_PROPERTY(QString analysisRequestId READ analysisRequestId NOTIFY analysisChanged)
    Q_PROPERTY(QString analysisSourceText READ analysisSourceText NOTIFY analysisChanged)
    Q_PROPERTY(QString analysisJson READ analysisJson NOTIFY analysisChanged)
    Q_PROPERTY(QUrl previewUrl READ previewUrl NOTIFY previewReady)
    Q_PROPERTY(bool darkMode READ darkMode NOTIFY themeChanged)
    Q_PROPERTY(bool closeLogOnSuccess READ closeLogOnSuccess NOTIFY logSettingsChanged)
    Q_PROPERTY(int defaultMoraDuration READ defaultMoraDuration NOTIFY synthesisDefaultsChanged)
    Q_PROPERTY(int defaultPauseDuration READ defaultPauseDuration NOTIFY synthesisDefaultsChanged)
    Q_PROPERTY(bool defaultApplyPitch READ defaultApplyPitch NOTIFY synthesisDefaultsChanged)
    Q_PROPERTY(QString synthesizeShortcut READ synthesizeShortcut NOTIFY shortcutSettingsChanged)
    Q_PROPERTY(QString saveProjectShortcut READ saveProjectShortcut NOTIFY shortcutSettingsChanged)
    Q_PROPERTY(QString reloadVoicebanksShortcut READ reloadVoicebanksShortcut NOTIFY shortcutSettingsChanged)
    Q_PROPERTY(QStringList logLines READ logLines NOTIFY logsChanged)
public:
    explicit Backend(QObject *parent = nullptr);
    ~Backend() override;
    bool connected() const { return m_handle != 0; }
    bool busy() const { return m_busy; }
    QString error() const { return m_error; }
    QVariantList voicebanks() const { return m_voicebanks; }
    QVariantList models() const { return m_models; }
    QVariantList renderers() const { return m_renderers; }
    QVariantList dictionaryEntries() const { return m_dictionaryEntries; }
    QString defaultRenderer() const { return m_defaultRenderer; }
    bool cudaAvailable() const { return m_cudaAvailable; }
    QString analysisRequestId() const { return m_analysisRequestId; }
    QString analysisSourceText() const { return m_analysisSourceText; }
    QString analysisJson() const { return m_analysisJson; }
    QUrl previewUrl() const { return m_previewUrl; }
    bool darkMode() const { return m_darkMode; }
    bool closeLogOnSuccess() const { return m_closeLogOnSuccess; }
    int defaultMoraDuration() const { return m_defaultMoraDuration; }
    int defaultPauseDuration() const { return m_defaultPauseDuration; }
    bool defaultApplyPitch() const { return m_defaultApplyPitch; }
    QString synthesizeShortcut() const { return m_synthesizeShortcut; }
    QString saveProjectShortcut() const { return m_saveProjectShortcut; }
    QString reloadVoicebanksShortcut() const { return m_reloadVoicebanksShortcut; }
    QStringList logLines() const { return m_logLines; }

    Q_INVOKABLE void initialize();
    Q_INVOKABLE void reloadVoicebanks();
    Q_INVOKABLE void analyze(const QString &text, const QString &requestId);
    Q_INVOKABLE void synthesize(const QVariantMap &request);
    Q_INVOKABLE bool savePreview(const QUrl &destination);
    Q_INVOKABLE QUrl defaultSaveFile(const QString &fileName) const;
    Q_INVOKABLE QUrl fileInDirectory(const QUrl &directory, const QString &fileName) const;
    Q_INVOKABLE bool saveProject(const QUrl &destination, const QVariantMap &project);
    Q_INVOKABLE QVariantMap loadProject(const QUrl &source);
    Q_INVOKABLE void setDarkMode(bool value);
    Q_INVOKABLE bool showNativeAboutDialog();
    Q_INVOKABLE void clearLogs();
    Q_INVOKABLE void setCloseLogOnSuccess(bool value);
    Q_INVOKABLE void setSynthesisDefaults(int moraDuration, int pauseDuration, bool applyPitch);
    Q_INVOKABLE void setShortcutSequences(const QString &synthesize, const QString &saveProject, const QString &reloadVoicebanks);
    Q_INVOKABLE void setDictionaryEntries(const QVariantList &entries);

signals:
    void connectedChanged();
    void busyChanged();
    void errorChanged();
    void metadataChanged();
    void analysisChanged();
    void previewReady();
    void themeChanged();
    void logSettingsChanged();
    void synthesisDefaultsChanged();
    void shortcutSettingsChanged();
    void dictionaryChanged();
    void logsChanged();

private:
    QVariantMap call(const QByteArray &method, const QVariantMap &request = {});
    void refreshMetadata();
    void setBusy(bool value);
    void setError(const QString &value);
    uintptr_t m_handle = 0;
    bool m_busy = false;
    QString m_error;
    QString m_previewPath;
    QTemporaryDir m_previewDirectory;
    QFutureSynchronizer<QVariantMap> m_activeCalls;
    int m_activeCallCount = 0;
    QVariantList m_voicebanks, m_models, m_renderers, m_dictionaryEntries;
    QString m_defaultRenderer;
    bool m_cudaAvailable = false;
    QString m_analysisRequestId, m_analysisSourceText, m_analysisJson;
    QUrl m_previewUrl;
    bool m_darkMode = false;
    bool m_closeLogOnSuccess = true;
    int m_defaultMoraDuration = 120;
    int m_defaultPauseDuration = 180;
    bool m_defaultApplyPitch = true;
    QString m_synthesizeShortcut;
    QString m_saveProjectShortcut;
    QString m_reloadVoicebanksShortcut;
    QStringList m_logLines;
    QHash<QString, quint64> m_analysisGenerations;
    quint64 m_nextAnalysisGeneration = 0;

    void appendLog(const QString &message);
};
