#include "selftest.h"

#include "backend.h"

#include <QEventLoop>
#include <QDebug>
#include <QFileInfo>
#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QMetaObject>
#include <QTemporaryDir>
#include <QTimer>
#include <QUrl>
#include <QVariantList>
#include <QVariantMap>

namespace {
constexpr int asyncTimeoutMS = 60000;

bool require(bool condition, const QString &message) {
    if (!condition)
        qCritical().noquote() << "self-test:" << message;
    return condition;
}

template<typename Signal, typename Trigger>
bool waitFor(Backend &backend, Signal signal, Trigger trigger, const QString &operation) {
    QEventLoop loop;
    QTimer timer;
    timer.setSingleShot(true);
    bool completed = false;
    QObject::connect(&backend, signal, &loop, [&] {
        completed = true;
        loop.quit();
    });
    QObject::connect(&timer, &QTimer::timeout, &loop, &QEventLoop::quit);
    trigger();
    timer.start(asyncTimeoutMS);
    loop.exec();
    return require(completed, operation + QStringLiteral(" timed out"))
        && require(backend.error().isEmpty(), operation + QStringLiteral(": ") + backend.error());
}

QString firstID(const QVariantList &items) {
    return items.isEmpty() ? QString() : items.first().toMap().value(QStringLiteral("id")).toString();
}

QString productionRenderer(const Backend &backend, const QVariantMap &model) {
    const QVariantList recommended = model.value(QStringLiteral("recommended_renderers")).toList();
    for (const QVariant &wantedValue : recommended) {
        const QString wanted = wantedValue.toString();
        for (const QVariant &rendererValue : backend.renderers()) {
            const QVariantMap renderer = rendererValue.toMap();
            if (renderer.value(QStringLiteral("id")).toString() == wanted
                    && renderer.value(QStringLiteral("acceleration")).toString() != QStringLiteral("cuda"))
                return wanted;
        }
    }
    return firstID(backend.renderers());
}

QVariantMap sampleProject(const QString &voicebankID, const QString &modelID, const QString &rendererID) {
    const QVariantMap utterance{
        {"text", QStringLiteral("こんにちは")}, {"voicebank_id", voicebankID},
        {"model_id", modelID}, {"renderer_id", rendererID}, {"alias_policy", "auto"},
        {"tone", "C4"}, {"color", ""}, {"mora_duration_ms", 120},
        {"pause_duration_ms", 180}, {"intonation", 1.0}, {"apply_pitch", true},
        {"pitch_points", QVariantList{}}, {"mora_durations_ms", QVariantList{}},
        {"mora_positions_ms", QVariantList{}}
    };
    return {{"format", "utautts-project"}, {"format_version", 5},
            {"utterances", QVariantList{utterance}}, {"selected_index", 0}};
}
}

int runSelfTest(Backend &backend, QObject *rootObject) {
    if (!require(backend.connected(), QStringLiteral("native backend is not connected"))
            || !require(!backend.voicebanks().isEmpty(), QStringLiteral("no bundled voicebank"))
            || !require(!backend.models().isEmpty(), QStringLiteral("no bundled prosody model"))
            || !require(!backend.renderers().isEmpty(), QStringLiteral("no bundled renderer")))
        return 1;

    QVariant interfaceResult;
    if (!require(rootObject != nullptr
                 && QMetaObject::invokeMethod(rootObject, "runInterfaceSelfTest",
                                              Q_RETURN_ARG(QVariant, interfaceResult)),
                 QStringLiteral("QML interface self-test could not be invoked"))
            || !require(interfaceResult.toString().isEmpty(),
                        QStringLiteral("QML interface self-test: ") + interfaceResult.toString()))
        return 1;

    QTemporaryDir temporary;
    if (!require(temporary.isValid(), QStringLiteral("temporary directory is unavailable")))
        return 1;

    const QString voicebankID = firstID(backend.voicebanks());
    const QVariantMap model = backend.models().first().toMap();
    const QString modelID = model.value(QStringLiteral("id")).toString();
    const QString rendererID = productionRenderer(backend, model);
    if (!require(!voicebankID.isEmpty() && !modelID.isEmpty() && !rendererID.isEmpty(),
                 QStringLiteral("metadata contains an empty id")))
        return 1;

    const QUrl projectURL = QUrl::fromLocalFile(temporary.filePath(QStringLiteral("smoke.utautts")));
    const QVariantMap project = sampleProject(voicebankID, modelID, rendererID);
    if (!require(backend.saveProject(projectURL, project), backend.error()))
        return 1;
    const QVariantMap loadedProject = backend.loadProject(projectURL);
    if (!require(!loadedProject.contains(QStringLiteral("_error"))
                 && loadedProject.value(QStringLiteral("format")).toString() == QStringLiteral("utautts-project")
                 && loadedProject.value(QStringLiteral("utterances")).toList().size() == 1,
                 QStringLiteral("project round trip failed")))
        return 1;

    if (!waitFor(backend, &Backend::analysisChanged,
                 [&] { backend.analyze(QStringLiteral("こんにちは"), QStringLiteral("self-test")); },
                 QStringLiteral("analysis")))
        return 1;
    const QJsonDocument analysis = QJsonDocument::fromJson(backend.analysisJson().toUtf8());
    if (!require(analysis.isObject() && !analysis.object().value(QStringLiteral("reading")).toString().isEmpty(),
                 QStringLiteral("analysis result is invalid")))
        return 1;

    const QVariantMap commonRequest{
        {"request_id", "self-test-prosody"}, {"text", QStringLiteral("こんにちは")},
        {"voicebank_id", voicebankID}, {"model_id", modelID}, {"renderer", rendererID},
        {"alias_policy", "auto"}, {"tone", "C4"}, {"mora_duration_ms", 120},
        {"pause_duration_ms", 180}, {"intonation_strength", 1.0}, {"apply_pitch", true}
    };
    if (!waitFor(backend, &Backend::prosodyChanged,
                 [&] { backend.predictProsody(commonRequest); }, QStringLiteral("prosody prediction")))
        return 1;
    const QJsonDocument prosody = QJsonDocument::fromJson(backend.prosodyJson().toUtf8());
    if (!require(prosody.isObject()
                 && !prosody.object().value(QStringLiteral("mora_durations_ms")).toArray().isEmpty(),
                 QStringLiteral("prosody result is invalid")))
        return 1;

    if (!waitFor(backend, &Backend::previewReady,
                 [&] { backend.synthesize(commonRequest); }, QStringLiteral("synthesis")))
        return 1;
    const QUrl wavURL = QUrl::fromLocalFile(temporary.filePath(QStringLiteral("smoke.wav")));
    if (!require(backend.savePreview(wavURL), backend.error())
            || !require(QFileInfo(wavURL.toLocalFile()).size() > 44, QStringLiteral("saved WAV is empty")))
        return 1;

    const QUrl exoURL = backend.writeDragExo(QUrl::fromLocalFile(temporary.path()), QVariantList{wavURL}, 30);
    if (!require(exoURL.isLocalFile() && QFileInfo::exists(exoURL.toLocalFile()),
                 QStringLiteral("exo export failed: ") + backend.error()))
        return 1;

    if (!require(!backend.loadProsodyPromptSet().contains(QStringLiteral("_error")),
                 QStringLiteral("prosody prompt set could not be loaded")))
        return 1;
    const QVariantMap acceptedRecord{
        {"text", QStringLiteral("あ")}, {"reading", QStringLiteral("ア")},
        {"morae", QVariantList{QStringLiteral("あ")}}, {"features", QVariantList{QVariantMap{}}},
        {"base_points_cents", QVariantList{0.0}}, {"manual_offsets_cents", QVariantList{0.0}},
        {"edit_mask", QVariantList{false}}, {"accepted", true}, {"status", "accepted"}
    };
    const QVariantMap session{{"format", "utautts-prosody-training-session"},
                              {"format_version", 1}, {"records", QVariantList{acceptedRecord}}};
    if (!require(backend.saveProsodyTrainingSession(session), backend.error())
            || !require(backend.loadProsodyTrainingSession().value(QStringLiteral("format")).toString()
                        == QStringLiteral("utautts-prosody-training-session"),
                        QStringLiteral("prosody training session round trip failed")))
        return 1;
    const QUrl datasetURL = QUrl::fromLocalFile(temporary.filePath(QStringLiteral("training.jsonl")));
    if (!require(backend.exportProsodyTrainingDataset(datasetURL, session), backend.error())
            || !require(QFileInfo::exists(datasetURL.toLocalFile()), QStringLiteral("training dataset export failed"))
            || !require(backend.clearProsodyTrainingSession(), backend.error()))
        return 1;

    backend.setDictionaryEntries(QVariantList{QVariantMap{{"surface", "UtauTTS"}, {"reading", "うたうてぃーてぃーえす"}}});
    if (!require(backend.dictionaryEntries().size() == 1 && !backend.dictionaryFingerprint().isEmpty(),
                 QStringLiteral("dictionary settings failed")))
        return 1;
    backend.setSynthesisDefaults(130, 190, false);
    backend.setShortcutSequences("Ctrl+Enter", "Ctrl+S", "Ctrl+O", "Ctrl+D", "Delete", "Ctrl+Z", "Ctrl+Y");
    if (!require(backend.defaultMoraDuration() == 130 && backend.defaultPauseDuration() == 190
                 && !backend.defaultApplyPitch() && backend.undoShortcut() == QStringLiteral("Ctrl+Z"),
                 QStringLiteral("application settings failed")))
        return 1;

    qInfo() << "UtauTTS self-test passed";
    return 0;
}
