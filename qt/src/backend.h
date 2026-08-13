#pragma once

#include <QObject>
#include <QFutureSynchronizer>
#include <QTemporaryDir>
#include <QUrl>
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
    Q_PROPERTY(QString defaultRenderer READ defaultRenderer NOTIFY metadataChanged)
    Q_PROPERTY(bool cudaAvailable READ cudaAvailable NOTIFY metadataChanged)
public:
    explicit Backend(QObject *parent = nullptr);
    ~Backend() override;
    bool connected() const { return m_handle != 0; }
    bool busy() const { return m_busy; }
    QString error() const { return m_error; }
    QVariantList voicebanks() const { return m_voicebanks; }
    QVariantList models() const { return m_models; }
    QVariantList renderers() const { return m_renderers; }
    QString defaultRenderer() const { return m_defaultRenderer; }
    bool cudaAvailable() const { return m_cudaAvailable; }

    Q_INVOKABLE void initialize();
    Q_INVOKABLE void reloadVoicebanks();
    Q_INVOKABLE void analyze(const QString &text, const QString &requestId);
    Q_INVOKABLE void synthesize(const QVariantMap &request);
    Q_INVOKABLE bool savePreview(const QUrl &destination);

signals:
    void connectedChanged();
    void busyChanged();
    void errorChanged();
    void metadataChanged();
    void analysisReady(const QString &requestId, const QString &sourceText,
                       const QVariantMap &analysis);
    void synthesisReady(const QUrl &audio, const QVariantMap &result);

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
    QVariantList m_voicebanks, m_models, m_renderers;
    QString m_defaultRenderer;
    bool m_cudaAvailable = false;
    quint64 m_analysisGeneration = 0;
};
