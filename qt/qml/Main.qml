pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQuick.Dialogs
import QtMultimedia

ApplicationWindow {
    id: window
    required property var injectedBackend
    required property var injectedLegalDocuments
    required property string injectedAppName
    required property url injectedRepositoryUrl
    width: 1240
    height: 800
    minimumWidth: 880
    minimumHeight: 600
    visible: true
    title: injectedAppName
    color: palette.window
    palette: Palette {
        window: window.darkMode ? "#202124" : "#f6f6f6"
        windowText: window.darkMode ? "#e8eaed" : "#202124"
        base: window.darkMode ? "#292a2d" : "#ffffff"
        alternateBase: window.darkMode ? "#303134" : "#f0f1f2"
        text: window.darkMode ? "#e8eaed" : "#202124"
        button: window.darkMode ? "#303134" : "#f0f1f2"
        buttonText: window.darkMode ? "#e8eaed" : "#202124"
        highlight: window.darkMode ? "#e8837d" : "#d35f6b"
        highlightedText: window.darkMode ? "#202124" : "#ffffff"
        placeholderText: window.darkMode ? "#aeb4ba" : "#697078"
        mid: window.darkMode ? "#5f6368" : "#aeb4ba"
    }

    property color accent: palette.highlight
    property color borderColor: palette.mid
    property color mutedText: palette.placeholderText
    readonly property url repositoryUrl: injectedRepositoryUrl
    readonly property var appBackend: injectedBackend
    readonly property bool darkMode: appBackend.darkMode
    readonly property var licenseDocuments: injectedLegalDocuments
    property int selectedIndex: 0
    property int nextUtteranceId: 1
    property int draggedUtteranceIndex: -1
    property string audioUtteranceId: ""
    property int audioRevision: -1
    property string pendingUtteranceId: ""
    property int pendingRevision: -1
    property string pendingProsodyRequestId: ""
    property string pendingProsodyUtteranceId: ""
    property int pendingProsodyRevision: -1
    property bool saveRequestPending: false
    property bool playbackRequested: false
    property string playbackError: ""
    property bool batchExportActive: false
    property int batchExportIndex: -1
    property int batchExportOriginalIndex: 0
    property int batchExportCompleted: 0
    property string batchExportMode: ""
    property var batchExportQueue: []
    property url batchExportDirectory
    property var dragExportFiles: []
    property string dragExportTarget: ""
    property bool dragExportSelectedOnly: false
    property bool dragExportReady: false
    property bool playbackQueueActive: false
    property var playbackQueue: []
    property int playbackQueueIndex: -1
    property bool projectDirty: false
    property bool metadataInitialized: false
    property bool closeAfterProjectSave: false
    property bool closeBypass: false

    Shortcut {
        sequence: window.qtShortcutSequence(window.appBackend.synthesizeShortcut)
        enabled: !settingsWindow.visible && !window.appBackend.busy && !window.batchExportActive
                 && utterances.count > 0 && window.current().content.trim().length > 0
        onActivated: window.synthesizeCurrent()
    }

    Shortcut {
        sequence: window.qtShortcutSequence(window.appBackend.saveProjectShortcut)
        enabled: !settingsWindow.visible && !window.appBackend.busy && !window.batchExportActive
        onActivated: window.openProjectSaveDialog()
    }

    Shortcut {
        sequence: window.qtShortcutSequence(window.appBackend.reloadVoicebanksShortcut)
        enabled: !settingsWindow.visible && !window.appBackend.busy && !window.batchExportActive
        onActivated: window.appBackend.reloadVoicebanks()
    }

    Shortcut {
        sequence: window.qtShortcutSequence(window.appBackend.addUtteranceShortcut)
        enabled: !settingsWindow.visible && !window.appBackend.busy && !window.batchExportActive
                 && !window.playbackQueueActive
        onActivated: window.addUtterance()
    }

    Shortcut {
        sequence: window.qtShortcutSequence(window.appBackend.removeUtteranceShortcut)
        enabled: !settingsWindow.visible && !window.appBackend.busy && !window.batchExportActive
                 && !window.playbackQueueActive
                 && utterances.count > 0
        onActivated: window.removeUtterance()
    }

    ListModel {
        id: utterances
    }

    Timer {
        id: analyzeTimer
        interval: 250
        onTriggered: {
            if (utterances.count && window.current().content.trim()) {
                const item = window.current();
                window.appBackend.analyze(item.content, item.utteranceId);
            }
        }
    }

    AudioOutput {
        id: previewAudioOutput
        volume: 1.0
        muted: false
    }

    MediaPlayer {
        id: player
        audioOutput: previewAudioOutput
        onMediaStatusChanged: {
            if (window.playbackRequested && (mediaStatus === MediaPlayer.LoadedMedia || mediaStatus === MediaPlayer.BufferedMedia)) {
                window.playbackRequested = false;
                play();
            } else if (window.playbackQueueActive && mediaStatus === MediaPlayer.EndOfMedia) {
                ++window.playbackQueueIndex;
                window.playNextPlaybackItem();
            }
        }
        onErrorOccurred: (error, errorString) => {
            window.playbackRequested = false;
            window.playbackError = errorString;
        }
    }

    FileDialog {
        id: saveDialog
        fileMode: FileDialog.SaveFile
        nameFilters: ["WAV音声 (*.wav)"]
        defaultSuffix: "wav"
        onAccepted: window.appBackend.savePreview(selectedFile)
    }

    FolderDialog {
        id: saveAllDialog
        onAccepted: window.startBatchExport(selectedFolder)
    }

    FolderDialog {
        id: dragSaveDialog
        onAccepted: window.openDragTargetSelectionWindow(selectedFolder)
    }

    DragTargetSelectionWindow {
        id: dragTargetSelectionWindow
        hostPalette: window.palette
        mutedText: window.mutedText
        exportDirectory: window.batchExportDirectory
        onExportRequested: target => {
            dragTargetSelectionWindow.close();
            window.startDragExport(target);
        }
    }

    DragSourceWindow {
        id: dragTargetWindow
        hostPalette: window.palette
        backend: window.appBackend
        files: window.dragExportFiles
        targetName: window.dragExportTarget
        exportDirectory: window.batchExportDirectory
        ready: window.dragExportReady
        accent: window.accent
        mutedText: window.mutedText
        onDragError: window.showAuxiliaryWindow(synthesisLogWindow)
    }

    SynthesisLogWindow {
        id: synthesisLogWindow
        hostPalette: window.palette
        backend: window.appBackend
    }

    SettingsWindow {
        id: settingsWindow
        hostWindow: window
        hostPalette: window.palette
        backend: window.appBackend
        onSaveRequested: window.saveSettings()
    }

    DictionaryWindow {
        id: dictionaryWindow
        hostWindow: window
        hostPalette: window.palette
        backend: window.appBackend
    }

    LicenseWindow {
        id: licenseWindow
        hostWindow: window
        hostPalette: window.palette
        documents: window.licenseDocuments
    }

    VoicebankDetailsWindow {
        id: voicebankDetailsWindow
        hostWindow: window
        hostPalette: window.palette
        backend: window.appBackend
    }

    FileDialog {
        id: projectSaveDialog
        fileMode: FileDialog.SaveFile
        nameFilters: ["UtauTTSプロジェクト (*.utautts)"]
        defaultSuffix: "utautts"
        onAccepted: window.saveProjectTo(selectedFile)
        onRejected: window.closeAfterProjectSave = false
    }

    FileDialog {
        id: projectOpenDialog
        fileMode: FileDialog.OpenFile
        nameFilters: ["UtauTTSプロジェクト (*.utautts)"]
        onAccepted: window.loadProjectFrom(selectedFile)
    }

    Dialog {
        id: closeWarningDialog
        title: "終了の確認"
        modal: true
        width: Math.min(window.width - 40, 460)
        anchors.centerIn: Overlay.overlay
        closePolicy: Popup.NoAutoClose

        contentItem: ColumnLayout {
            spacing: 12

            Label {
                Layout.fillWidth: true
                text: window.appBackend.busy || window.batchExportActive
                      ? "音声合成が実行中です。終了しますか？"
                      : "未保存の変更があります。保存せずに終了しますか？"
                wrapMode: Text.WordWrap
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: 8

                Item {
                    Layout.fillWidth: true
                }

                Button {
                    text: "キャンセル"
                    onClicked: closeWarningDialog.close()
                }

                Button {
                    text: "保存して終了"
                    enabled: window.projectDirty && !window.appBackend.busy && !window.batchExportActive
                    onClicked: {
                        closeWarningDialog.close();
                        window.closeAfterProjectSave = true;
                        window.openProjectSaveDialog();
                    }
                }

                Button {
                    text: "保存せずに終了"
                    onClicked: {
                        closeWarningDialog.close();
                        window.quitWithoutWarning();
                    }
                }
            }
        }
    }

    MessageDialog {
        id: shortcutConflictDialog
        title: "ショートカット設定"
        text: "同じショートカットが複数の機能に割り当てられています。別のキーを設定してください。"
        buttons: MessageDialog.Ok
    }

    MessageDialog {
        id: projectLoadErrorDialog
        title: "プロジェクトを開けません"
        buttons: MessageDialog.Ok
    }

    MessageDialog {
        id: aboutDialog
        title: "UtauTTSについて"
        text: "UtauTTS " + Qt.application.version + "\n\nDeveloped by yh（@2237yh）\nTesting by アアアアアアア（@a7_riri）"
        informativeText: "UTAUボイスバンクの原音接続と、深層学習による日本語イントネーションを組み合わせたTTS"
        buttons: MessageDialog.Ok
    }

    Connections {
        target: window.appBackend

        function onMetadataChanged() {
            const suppressDirty = !window.metadataInitialized;
            window.assignDefaultVoicebank(suppressDirty);
            window.assignDefaultSynthesisSettings(suppressDirty);
            window.metadataInitialized = true;
            if (suppressDirty)
                window.projectDirty = false;
        }

        function onAnalysisChanged() {
            const requestId = window.appBackend.analysisRequestId;
            const sourceText = window.appBackend.analysisSourceText;
            const index = window.utteranceIndex(requestId);
            if (index < 0 || utterances.get(index).content !== sourceText)
                return;
            const analysis = JSON.parse(window.appBackend.analysisJson);
            const old = utterances.get(index);
            const oldPoints = window.decodeSequence(old.pointsJson);
            const oldDurations = window.decodeSequence(old.moraDurationsJson);
            const oldPositions = window.decodeSequence(old.moraPositionsJson);
            const morae = window.copySequence(analysis.morae);
            const values = [];
            const durations = [];
            const positions = oldPositions.length === morae.length ? oldPositions.slice() : [];
            for (let i = 0; i < morae.length; ++i)
                values.push(i < oldPoints.length ? oldPoints[i] : 0);
            for (let i = 0; i < morae.length; ++i)
                durations.push(i < oldDurations.length ? oldDurations[i] : 0);
            utterances.setProperty(index, "reading", analysis.reading);
            utterances.setProperty(index, "moraeJson", JSON.stringify(morae));
            utterances.setProperty(index, "pointsJson", JSON.stringify(values));
            utterances.setProperty(index, "moraDurationsJson", JSON.stringify(durations));
            utterances.setProperty(index, "moraPositionsJson", JSON.stringify(positions));
            utterances.setProperty(index, "autoPointsJson", "[]");
            utterances.setProperty(index, "autoMoraDurationsJson", "[]");
            utterances.setProperty(index, "autoMoraPositionsJson", "[]");
            if (index === window.selectedIndex) {
                pitchEditor.points = values.slice();
                pitchEditor.autoPoints = [];
                pitchEditor.morae = morae.slice();
                pitchEditor.moraDurations = durations.slice();
                pitchEditor.moraPositions = positions.slice();
            }
            window.requestProsodyPreview(index);
        }

        function onProsodyChanged() {
            if (window.appBackend.prosodyRequestId !== window.pendingProsodyRequestId)
                return;
            const index = window.utteranceIndex(window.pendingProsodyUtteranceId);
            if (index < 0 || utterances.get(index).revision !== window.pendingProsodyRevision)
                return;
            let result;
            try {
                result = JSON.parse(window.appBackend.prosodyJson);
            } catch (error) {
                return;
            }
            const item = utterances.get(index);
            const automaticPoints = window.copySequence(result.pitch_points);
            const automaticDurations = window.copySequence(result.mora_durations_ms);
            const automaticPositions = window.copySequence(result.mora_positions_ms);
            utterances.setProperty(index, "autoPointsJson", JSON.stringify(automaticPoints));
            utterances.setProperty(index, "autoMoraDurationsJson", JSON.stringify(automaticDurations));
            utterances.setProperty(index, "autoMoraPositionsJson", JSON.stringify(automaticPositions));
            if (index === window.selectedIndex) {
                pitchEditor.autoPoints = automaticPoints.slice();
                pitchEditor.moraDurations = window.hasManualMoraDurations(item)
                        ? window.decodeSequence(item.moraDurationsJson) : automaticDurations.slice();
                pitchEditor.moraPositions = window.hasManualMoraDurations(item)
                        ? window.decodeSequence(item.moraPositionsJson) : automaticPositions.slice();
            }
        }

        function onSynthesisChanged() {
            const pendingId = window.pendingUtteranceId;
            const pendingRevision = window.pendingRevision;
            const index = window.utteranceIndex(pendingId);
            if (index < 0)
                return;
            const item = utterances.get(index);
            if (item.revision !== pendingRevision)
                return;
            let result;
            try {
                result = JSON.parse(window.appBackend.synthesisJson);
            } catch (error) {
                return;
            }
            const automaticPoints = window.copySequence(result.pitch_points);
            const automaticDurations = window.copySequence(result.mora_durations_ms);
            const automaticPositions = window.copySequence(result.mora_positions_ms);
            utterances.setProperty(index, "autoPointsJson", JSON.stringify(automaticPoints));
            utterances.setProperty(index, "autoMoraDurationsJson", JSON.stringify(automaticDurations));
            utterances.setProperty(index, "autoMoraPositionsJson", JSON.stringify(automaticPositions));
            if (index === window.selectedIndex) {
                pitchEditor.autoPoints = automaticPoints.slice();
                pitchEditor.moraDurations = window.hasManualMoraDurations(item)
                        ? window.decodeSequence(item.moraDurationsJson) : automaticDurations.slice();
                pitchEditor.moraPositions = window.hasManualMoraDurations(item)
                        ? window.decodeSequence(item.moraPositionsJson) : automaticPositions.slice();
            }
        }

        function onPreviewReady() {
            const audio = window.appBackend.previewUrl;
            const pendingId = window.pendingUtteranceId;
            const pendingRevision = window.pendingRevision;
            const index = window.utteranceIndex(pendingId);
            if (window.batchExportActive) {
                if (index < 0 || utterances.get(index).revision !== pendingRevision) {
                    window.finishBatchExport(false);
                    return;
                }
                const fileName = window.batchExportMode === "drag"
                        ? window.dragAudioFileName(utterances.get(index), index)
                        : window.audioFileName(utterances.get(index));
                const destination = window.appBackend.fileInDirectory(
                            window.batchExportDirectory, fileName);
                window.pendingUtteranceId = "";
                window.pendingRevision = -1;
                if (!destination.toString().length || !window.appBackend.savePreview(destination)) {
                    window.finishBatchExport(false);
                    return;
                }
                ++window.batchExportCompleted;
                if (window.batchExportMode === "drag")
                    window.dragExportFiles.push(destination);
                Qt.callLater(window.synthesizeBatchItem);
                return;
            }
            if (window.saveRequestPending) {
                if (index < 0 || utterances.get(index).revision !== pendingRevision) {
                    window.saveRequestPending = false;
                    window.pendingUtteranceId = "";
                    window.pendingRevision = -1;
                    return;
                }
                window.saveRequestPending = false;
                window.pendingUtteranceId = "";
                window.pendingRevision = -1;
                saveDialog.currentFile = window.appBackend.defaultSaveFile(window.audioFileName(utterances.get(index)));
                if (window.appBackend.closeLogOnSuccess)
                    synthesisLogWindow.close();
                saveDialog.open();
                return;
            }
            if (window.playbackQueueActive) {
                if (index < 0 || index !== window.selectedIndex || utterances.get(index).revision !== window.pendingRevision) {
                    window.stopPlaybackQueue();
                    return;
                }
                window.audioUtteranceId = window.pendingUtteranceId;
                window.audioRevision = window.pendingRevision;
                window.pendingUtteranceId = "";
                window.pendingRevision = -1;
                window.playbackError = "";
                window.playbackRequested = true;
                player.stop();
                player.source = audio;
                player.play();
                return;
            }
            if (index < 0 || index !== window.selectedIndex || utterances.get(index).revision !== window.pendingRevision) {
                window.pendingUtteranceId = "";
                window.pendingRevision = -1;
                return;
            }
            window.audioUtteranceId = window.pendingUtteranceId;
            window.audioRevision = window.pendingRevision;
            window.pendingUtteranceId = "";
            window.pendingRevision = -1;
            window.playbackError = "";
            window.playbackRequested = true;
            if (window.appBackend.closeLogOnSuccess)
                synthesisLogWindow.close();
            player.stop();
            player.source = audio;
            player.play();
        }

        function onErrorChanged() {
            if (window.batchExportActive && window.pendingUtteranceId.length && window.appBackend.error.length)
                window.finishBatchExport(false);
            else if (window.playbackQueueActive && window.pendingUtteranceId.length && window.appBackend.error.length)
                window.stopPlaybackQueue();
            else if (window.saveRequestPending && window.pendingUtteranceId.length && window.appBackend.error.length) {
                window.saveRequestPending = false;
                window.pendingUtteranceId = "";
                window.pendingRevision = -1;
            }
        }
    }

    Component.onCompleted: addUtterance(false)

    onClosing: close => {
        if (window.closeBypass) {
            window.closeBypass = false;
            return;
        }
        if (!window.projectDirty && !window.appBackend.busy && !window.batchExportActive)
            return;
        close.accepted = false;
        closeWarningDialog.open();
    }

    menuBar: MenuBar {
        Menu {
            title: "ファイル"
            MenuItem {
                text: "プロジェクトを開く..."
                enabled: !window.appBackend.busy && !window.batchExportActive
                onTriggered: projectOpenDialog.open()
            }
            MenuItem {
                text: "プロジェクトを保存..."
                enabled: !window.appBackend.busy && !window.batchExportActive
                onTriggered: window.openProjectSaveDialog()
            }
            MenuSeparator {}
            MenuItem {
                text: "WAVを保存..."
                enabled: utterances.count > 0 && !window.appBackend.busy && !window.batchExportActive && window.current().content.trim().length > 0
                onTriggered: window.saveCurrentAudio()
            }
            MenuItem {
                text: "WAVをすべて保存..."
                enabled: utterances.count > 0 && !window.appBackend.busy && !window.batchExportActive
                onTriggered: window.openSaveAllDialog()
            }
            MenuSeparator {}
            MenuItem {
                text: "選択中の音声をドラッグ＆ドロップ..."
                enabled: utterances.count > 0 && !window.appBackend.busy && !window.batchExportActive && window.current().content.trim().length > 0
                onTriggered: window.openDragExportDialog(true)
            }
            MenuItem {
                text: "全テキストの音声をドラッグ＆ドロップ..."
                enabled: utterances.count > 0 && !window.appBackend.busy && !window.batchExportActive
                onTriggered: window.openDragExportDialog(false)
            }
            MenuItem {
                text: "音源を再読込"
                enabled: !window.appBackend.busy
                onTriggered: window.appBackend.reloadVoicebanks()
            }
            MenuSeparator {}
            MenuItem {
                text: "終了"
                onTriggered: Qt.quit()
            }
        }
        Menu {
            title: "再生"
            MenuItem {
                text: "選択中のテキストを再生"
                enabled: utterances.count > 0 && !window.appBackend.busy && !window.batchExportActive
                         && !window.playbackQueueActive && window.current().content.trim().length > 0
                onTriggered: window.synthesizeCurrent()
            }
            MenuItem {
                text: "全テキストを再生"
                enabled: !window.appBackend.busy && !window.batchExportActive && !window.playbackQueueActive
                         && window.hasPlayableTextFrom(0)
                onTriggered: window.startPlaybackQueue(0)
            }
            MenuItem {
                text: "選択中のテキストより先全てを再生"
                enabled: !window.appBackend.busy && !window.batchExportActive && !window.playbackQueueActive
                         && window.hasPlayableTextFrom(window.selectedIndex)
                onTriggered: window.startPlaybackQueue(window.selectedIndex)
            }
            MenuSeparator {}
            MenuItem {
                text: "もう一度再生"
                enabled: !window.appBackend.busy && !window.batchExportActive && !window.playbackQueueActive
                         && window.hasCachedAudio()
                onTriggered: window.replayCachedAudio()
            }
        }
        Menu {
            title: "設定"
            MenuItem {
                text: "設定..."
                onTriggered: {
                    window.showAuxiliaryWindow(settingsWindow);
                    settingsWindow.loadCurrent();
                }
            }
            MenuItem {
                text: "辞書設定..."
                onTriggered: window.openDictionarySettings()
            }
        }
        Menu {
            title: "ヘルプ"
            MenuItem {
                text: "UtauTTSについて..."
                onTriggered: {
                    if (!window.appBackend.showNativeAboutDialog())
                        aboutDialog.open();
                }
            }
            MenuItem {
                text: "GitHubリポジトリ"
                onTriggered: Qt.openUrlExternally(window.repositoryUrl)
            }
            MenuItem {
                text: "ライセンス..."
                onTriggered: window.showAuxiliaryWindow(licenseWindow)
            }
            MenuItem {
                text: "ボイスバンクの詳細..."
                enabled: window.appBackend.voicebanks.length > 0
                onTriggered: window.showVoicebankDetails()
            }
        }
    }

    SplitView {
        anchors.fill: parent
        orientation: Qt.Vertical

        SplitView {
            SplitView.fillHeight: true
            orientation: Qt.Horizontal

            Pane {
                SplitView.fillWidth: true
                SplitView.minimumWidth: 560
                padding: 10
                background: Rectangle {
                    color: window.palette.window
                }

                ListView {
                    id: utteranceList
                    anchors.fill: parent
                    model: utterances
                    clip: true
                    spacing: 4
                    boundsBehavior: Flickable.StopAtBounds
                    bottomMargin: 64
                    ScrollBar.vertical: ScrollBar {
                        id: utteranceScrollBar
                        policy: ScrollBar.AlwaysOn
                    }

                    delegate: Item {
                        id: card
                        required property int index
                        required property string content
                        required property string voicebankId
                        required property string imagePath
                        property alias textEditor: utteranceEditor

                        width: Math.max(0, utteranceList.width - 14 - 2)
                        height: 46

                        RowLayout {
                            anchors.fill: parent
                            spacing: 6

                            Rectangle {
                                id: imageHandle
                                Layout.preferredWidth: 42
                                Layout.preferredHeight: 42
                                radius: 2
                                color: window.palette.alternateBase
                                border.color: card.index === window.selectedIndex ? window.accent : window.borderColor

                                Image {
                                    anchors.fill: parent
                                    anchors.margins: 2
                                    source: window.localImageUrl(card.imagePath)
                                    fillMode: Image.PreserveAspectFit
                                    asynchronous: true
                                }
                                Label {
                                    anchors.centerIn: parent
                                    visible: !card.imagePath
                                    text: "音源"
                                    color: window.mutedText
                                    font.pixelSize: 9
                                }

                                DragHandler {
                                    id: imageDrag
                                    target: dragProxy
                                    onActiveChanged: {
                                        if (active) {
                                            window.selectUtterance(card.index);
                                            window.draggedUtteranceIndex = card.index;
                                            dragProxy.x = imageHandle.x;
                                            dragProxy.y = imageHandle.y;
                                        } else
                                            window.draggedUtteranceIndex = -1;
                                    }
                                }
                                ToolTip.visible: imageHover.hovered && !imageDrag.active
                                ToolTip.text: window.voicebankName(card.voicebankId) + "\nドラッグして並べ替え"
                                HoverHandler {
                                    id: imageHover
                                }
                            }

                            TextField {
                                id: utteranceEditor
                                Layout.fillWidth: true
                                Layout.preferredHeight: 42
                                text: card.content
                                font.pixelSize: 16
                                placeholderText: "文章を入力"
                                selectByMouse: true

                                onActiveFocusChanged: {
                                    if (activeFocus)
                                        window.selectUtterance(card.index);
                                }
                                Keys.priority: Keys.BeforeItem
                                Keys.onPressed: event => {
                                    if (event.key === Qt.Key_Delete
                                            && event.modifiers === Qt.NoModifier
                                            && window.qtShortcutSequence(window.appBackend.removeUtteranceShortcut).toLowerCase() === "delete"
                                            && !settingsWindow.visible
                                            && !window.appBackend.busy
                                            && !window.batchExportActive
                                            && !window.playbackQueueActive) {
                                        event.accepted = true;
                                        window.removeUtterance();
                                    }
                                }
                                onTextChanged: {
                                    if (card.index >= utterances.count || utterances.get(card.index).content === text)
                                        return;
                                    window.updateUtteranceText(card.index, text);
                                }
                            }

                            ToolButton {
                                text: "⋮"
                                visible: card.index === window.selectedIndex
                                onClicked: cardMenu.open()

                                Menu {
                                    id: cardMenu
                                    y: parent.height
                                    MenuItem {
                                        text: "上へ移動"
                                        enabled: card.index > 0
                                        onTriggered: {
                                            window.selectUtterance(card.index);
                                            window.moveUtterance(-1);
                                        }
                                    }
                                    MenuItem {
                                        text: "下へ移動"
                                        enabled: card.index < utterances.count - 1
                                        onTriggered: {
                                            window.selectUtterance(card.index);
                                            window.moveUtterance(1);
                                        }
                                    }
                                    MenuSeparator {}
                                    MenuItem {
                                        text: "削除"
                                        enabled: true
                                        onTriggered: {
                                            window.selectUtterance(card.index);
                                            window.removeUtterance();
                                        }
                                    }
                                }
                            }
                        }

                        Rectangle {
                            id: dragProxy
                            width: 42
                            height: 42
                            radius: 2
                            visible: imageDrag.active
                            color: window.palette.alternateBase
                            border.color: window.accent
                            opacity: 0.8
                            z: 20
                            Drag.active: imageDrag.active
                            Drag.source: card
                            Drag.keys: ["utterance"]
                            Drag.hotSpot.x: width / 2
                            Drag.hotSpot.y: height / 2

                            Image {
                                anchors.fill: parent
                                anchors.margins: 2
                                source: window.localImageUrl(card.imagePath)
                                fillMode: Image.PreserveAspectFit
                            }
                        }

                        DropArea {
                            anchors.fill: parent
                            keys: ["utterance"]
                            onEntered: drag => {
                                if (!drag.source)
                                    return;
                                const from = window.draggedUtteranceIndex;
                                const to = card.index;
                                if (from < 0 || to < 0 || from === to)
                                    return;
                                window.clearPlayback();
                                utterances.move(from, to, 1);
                                window.selectedIndex = to;
                                window.draggedUtteranceIndex = to;
                                window.projectDirty = true;
                            }
                        }
                    }
                }

                RoundButton {
                    id: addButton
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.rightMargin: 24
                    anchors.bottomMargin: 8
                    width: 48
                    height: 48
                    highlighted: true
                    z: 2
                    contentItem: Canvas {
                        id: addIcon
                        anchors.fill: parent
                        property color iconColor: addButton.palette.buttonText
                        onIconColorChanged: requestPaint()
                        onPaint: {
                            const context = getContext("2d");
                            context.clearRect(0, 0, width, height);
                            context.strokeStyle = iconColor;
                            context.lineWidth = 2.4;
                            context.lineCap = "round";
                            context.beginPath();
                            context.moveTo(width * 0.3, height * 0.5);
                            context.lineTo(width * 0.7, height * 0.5);
                            context.moveTo(width * 0.5, height * 0.3);
                            context.lineTo(width * 0.5, height * 0.7);
                            context.stroke();
                        }
                    }
                    onClicked: window.addUtterance()
                    ToolTip.visible: hovered
                    ToolTip.text: "追加"
                }
            }

            Pane {
                SplitView.preferredWidth: 268
                SplitView.minimumWidth: 238
                SplitView.maximumWidth: 340
                padding: 14
                background: Rectangle {
                    color: window.palette.window
                    border.color: window.borderColor
                }

                ScrollView {
                    id: parameterScroll
                    anchors.fill: parent
                    contentWidth: availableWidth
                    ScrollBar.vertical.policy: ScrollBar.AsNeeded

                    ColumnLayout {
                        width: Math.max(0, parameterScroll.availableWidth - 14)
                        spacing: 12

                        Label {
                            text: "音源"
                            font.pixelSize: 12
                            color: window.mutedText
                        }
                        ComboBox {
                            id: voiceCombo
                            Layout.fillWidth: true
                            model: window.appBackend.voicebanks
                            textRole: "name"
                            valueRole: "id"
                            onActivated: {
                                window.updateSetting("voicebankId", currentValue);
                                const voice = window.voicebankById(currentValue);
                                utterances.setProperty(window.selectedIndex, "imagePath", voice ? voice.image_path : "");
                            }
                        }

                        Label {
                            text: "抑揚モデル"
                            font.pixelSize: 12
                            color: window.mutedText
                        }
                        ComboBox {
                            id: modelCombo
                            Layout.fillWidth: true
                            model: [
                                {
                                    id: "none",
                                    display_name: "なし"
                                }
                            ].concat(window.appBackend.models)
                            textRole: "display_name"
                            valueRole: "id"
                            onActivated: {
                                window.updateSetting("modelId", currentValue);
                                const model = window.modelById(currentValue);
                                const renderer = window.preferredRendererForModel(model);
                                if (renderer) {
                                    window.updateSetting("renderer", renderer);
                                    window.selectCombo(rendererCombo, renderer);
                                }
                            }
                        }

                        Label {
                            text: "Renderer"
                            font.pixelSize: 12
                            color: window.mutedText
                        }
                        ComboBox {
                            id: rendererCombo
                            Layout.fillWidth: true
                            model: window.appBackend.renderers
                            textRole: "display_name"
                            valueRole: "id"
                            onActivated: window.updateSetting("renderer", currentValue)
                        }
                        Label {
                            Layout.fillWidth: true
                            text: window.appBackend.cudaAvailable ? "CUDA GPUを検出しました" : "CPUモード"
                            color: window.mutedText
                            font.pixelSize: 11
                        }
                        Label {
                            Layout.fillWidth: true
                            visible: window.appBackend.error.length > 0 || window.playbackError.length > 0
                            text: window.appBackend.error.length > 0 ? window.appBackend.error : window.playbackError
                            color: window.palette.text
                            wrapMode: Text.Wrap
                            font.pixelSize: 11
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Label {
                                text: "音高"
                                Layout.fillWidth: true
                            }
                            TextField {
                                id: toneField
                                Layout.preferredWidth: 72
                                horizontalAlignment: TextInput.AlignRight
                                text: "C4"
                                onEditingFinished: window.updateSetting("tone", text)
                            }
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Label {
                                text: "抑揚"
                                Layout.fillWidth: true
                            }
                            SpinBox {
                                id: intonationInput
                                Layout.preferredWidth: 86
                                from: 0
                                to: 200
                                stepSize: 5
                                editable: true
                                value: Math.round(intonationSlider.value * 100)
                                textFromValue: value => (value / 100).toFixed(2)
                                valueFromText: text => Math.round(parseFloat(text) * 100)
                                onValueModified: {
                                    intonationSlider.value = value / 100;
                                    window.updateSetting("intonation", value / 100);
                                }
                            }
                        }
                        Item {
                            Layout.fillWidth: true
                            Layout.preferredHeight: intonationSlider.implicitHeight
                            Slider {
                                id: intonationSlider
                                anchors.fill: parent
                                from: 0
                                to: 2
                                stepSize: .05
                                onMoved: {
                                    intonationInput.value = Math.round(value * 100);
                                    window.updateSetting("intonation", value);
                                }
                            }
                            MouseArea {
                                anchors.fill: parent
                                cursorShape: Qt.PointingHandCursor
                                onPressed: mouse => updateAt(mouse.x)
                                onPositionChanged: mouse => {
                                    if (pressed)
                                        updateAt(mouse.x);
                                }
                                onDoubleClicked: window.resetIntonation()
                                function updateAt(x) {
                                    const fraction = Math.max(0, Math.min(1, x / width));
                                    const value = Math.round((intonationSlider.from + fraction * (intonationSlider.to - intonationSlider.from)) / intonationSlider.stepSize) * intonationSlider.stepSize;
                                    intonationSlider.value = value;
                                    intonationInput.value = Math.round(value * 100);
                                    window.updateSetting("intonation", value);
                                }
                            }
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Label {
                                text: "モーラ長"
                                Layout.fillWidth: true
                            }
                            SpinBox {
                                id: moraInput
                                Layout.preferredWidth: 96
                                from: 60
                                to: 300
                                stepSize: 5
                                editable: true
                                value: Math.round(moraSlider.value)
                                textFromValue: value => value + " ms"
                                valueFromText: text => parseInt(text)
                                onValueModified: {
                                    moraSlider.value = value;
                                    window.updateSetting("moraDuration", value);
                                }
                            }
                        }
                        Item {
                            Layout.fillWidth: true
                            Layout.preferredHeight: moraSlider.implicitHeight
                            Slider {
                                id: moraSlider
                                anchors.fill: parent
                                from: 60
                                to: 300
                                stepSize: 5
                                onMoved: {
                                    window.updateSetting("moraDuration", value);
                                    moraInput.value = value;
                                }
                            }
                            MouseArea {
                                anchors.fill: parent
                                cursorShape: Qt.PointingHandCursor
                                onPressed: mouse => updateAt(mouse.x)
                                onPositionChanged: mouse => {
                                    if (pressed)
                                        updateAt(mouse.x);
                                }
                                onDoubleClicked: window.resetMoraDuration()
                                function updateAt(x) {
                                    const fraction = Math.max(0, Math.min(1, x / width));
                                    const value = Math.round((moraSlider.from + fraction * (moraSlider.to - moraSlider.from)) / moraSlider.stepSize) * moraSlider.stepSize;
                                    moraSlider.value = value;
                                    moraInput.value = value;
                                    window.updateSetting("moraDuration", value);
                                }
                            }
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Label {
                                text: "休止長"
                                Layout.fillWidth: true
                            }
                            SpinBox {
                                id: pauseInput
                                Layout.preferredWidth: 96
                                from: 0
                                to: 800
                                stepSize: 10
                                editable: true
                                value: Math.round(pauseSlider.value)
                                textFromValue: value => value + " ms"
                                valueFromText: text => parseInt(text)
                                onValueModified: {
                                    pauseSlider.value = value;
                                    window.updateSetting("pauseDuration", value);
                                }
                            }
                        }
                        Item {
                            Layout.fillWidth: true
                            Layout.preferredHeight: pauseSlider.implicitHeight
                            Slider {
                                id: pauseSlider
                                anchors.fill: parent
                                from: 0
                                to: 800
                                stepSize: 10
                                onMoved: {
                                    window.updateSetting("pauseDuration", value);
                                    pauseInput.value = value;
                                }
                            }
                            MouseArea {
                                anchors.fill: parent
                                cursorShape: Qt.PointingHandCursor
                                onPressed: mouse => updateAt(mouse.x)
                                onPositionChanged: mouse => {
                                    if (pressed)
                                        updateAt(mouse.x);
                                }
                                onDoubleClicked: window.resetPauseDuration()
                                function updateAt(x) {
                                    const fraction = Math.max(0, Math.min(1, x / width));
                                    const value = Math.round((pauseSlider.from + fraction * (pauseSlider.to - pauseSlider.from)) / pauseSlider.stepSize) * pauseSlider.stepSize;
                                    pauseSlider.value = value;
                                    pauseInput.value = value;
                                    window.updateSetting("pauseDuration", value);
                                }
                            }
                        }

                        Item {
                            Layout.fillHeight: true
                        }
                    }
                }
            }
        }

        Pane {
            SplitView.preferredHeight: 238
            SplitView.minimumHeight: 150
            padding: 0
            background: Rectangle {
                color: window.palette.window
                border.color: window.borderColor
            }

            ColumnLayout {
                anchors.fill: parent
                spacing: 0

                RowLayout {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 38
                    Layout.leftMargin: 12
                    Layout.rightMargin: 10
                    Label {
                        text: "イントネーション"
                        font.pixelSize: 12
                    }
                    Item {
                        Layout.fillWidth: true
                    }
                    Label {
                        text: "±300 cent"
                        color: window.mutedText
                        font.pixelSize: 11
                    }
                }
                Rectangle {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 1
                    color: window.borderColor
                }
                PitchEditor {
                    id: pitchEditor
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    Layout.leftMargin: 12
                    Layout.rightMargin: 12
                    accentColor: window.accent
                    axisColor: window.palette.mid
                    gridColor: window.palette.alternateBase
                    labelColor: window.palette.text
                    defaultMoraDuration: window.appBackend.defaultMoraDuration
                    defaultPauseDuration: window.appBackend.defaultPauseDuration
                    onPointsEdited: points => window.updatePitchPoints(points)
                    onMoraDurationsEdited: durations => window.updateMoraDurations(durations)
                    onMoraPositionsEdited: positions => window.updateMoraPositions(positions)
                }
                Item {
                    id: pitchHorizontalScrollBar
                    Layout.fillWidth: true
                    Layout.preferredHeight: 14
                    Layout.leftMargin: 12
                    Layout.rightMargin: 12
                    visible: pitchEditor.horizontalMaximum > 0

                    Rectangle {
                        id: pitchScrollTrack
                        anchors.verticalCenter: parent.verticalCenter
                        width: parent.width
                        height: 4
                        radius: height / 2
                        color: window.palette.mid
                    }
                    Rectangle {
                        id: pitchScrollThumb
                        readonly property real minimumWidth: 28
                        width: Math.max(minimumWidth, pitchScrollTrack.width * pitchEditor.horizontalVisibleRatio)
                        height: 10
                        radius: height / 2
                        y: (parent.height - height) / 2
                        x: (pitchScrollTrack.width - width) * pitchEditor.horizontalPosition
                        color: window.accent
                    }
                    MouseArea {
                        anchors.fill: parent
                        cursorShape: Qt.SizeHorCursor
                        onPressed: mouse => setOffset(mouse.x)
                        onPositionChanged: mouse => {
                            if (pressed)
                                setOffset(mouse.x);
                        }
                        function setOffset(x) {
                            const travel = pitchScrollTrack.width - pitchScrollThumb.width;
                            if (travel <= 0)
                                return;
                            const position = Math.max(0, Math.min(1, (x - pitchScrollThumb.width / 2) / travel));
                            pitchEditor.horizontalOffset = position * pitchEditor.horizontalMaximum;
                        }
                    }
                }
                RowLayout {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 52
                    Layout.leftMargin: 10
                    Layout.rightMargin: 10
                    spacing: 10

                    RoundButton {
                        id: playbackButton
                        Layout.preferredWidth: 42
                        Layout.preferredHeight: 42
                        highlighted: true
                        contentItem: Canvas {
                            id: playbackIcon
                            anchors.fill: parent
                            property int iconState: window.appBackend.busy ? 0 : player.playbackState === MediaPlayer.PlayingState ? 1 : 2
                            property color iconColor: playbackButton.palette.buttonText
                            onIconStateChanged: requestPaint()
                            onIconColorChanged: requestPaint()
                            onPaint: {
                                const context = getContext("2d");
                                context.clearRect(0, 0, width, height);
                                context.fillStyle = iconColor;
                                if (iconState === 0) {
                                    const radius = Math.max(1.5, width * 0.055);
                                    context.beginPath();
                                    context.arc(width * 0.35, height * 0.5, radius, 0, Math.PI * 2);
                                    context.arc(width * 0.5, height * 0.5, radius, 0, Math.PI * 2);
                                    context.arc(width * 0.65, height * 0.5, radius, 0, Math.PI * 2);
                                    context.fill();
                                } else if (iconState === 1) {
                                    context.fillRect(width * 0.34, height * 0.3, width * 0.11, height * 0.4);
                                    context.fillRect(width * 0.55, height * 0.3, width * 0.11, height * 0.4);
                                } else {
                                    context.beginPath();
                                    context.moveTo(width * 0.39, height * 0.28);
                                    context.lineTo(width * 0.39, height * 0.72);
                                    context.lineTo(width * 0.7, height * 0.5);
                                    context.closePath();
                                    context.fill();
                                }
                            }
                        }
                        enabled: !window.batchExportActive && (player.playbackState === MediaPlayer.PlayingState || window.hasCurrentAudio() || (!window.appBackend.busy && utterances.count && window.current().content.trim().length > 0))
                        onClicked: {
                            if (player.playbackState === MediaPlayer.PlayingState)
                                player.pause();
                            else if (window.hasCurrentAudio()) {
                                if (player.duration > 0 && player.position >= player.duration - 1)
                                    player.position = 0;
                                player.play();
                            } else
                                window.synthesizeCurrent();
                        }
                        ToolTip.visible: hovered
                        ToolTip.text: window.appBackend.error.length ? window.appBackend.error : window.playbackError.length ? window.playbackError : player.playbackState === MediaPlayer.PlayingState ? "一時停止" : "生成して再生"
                    }
                    Slider {
                        Layout.fillWidth: true
                        from: 0
                        to: Math.max(1, player.duration)
                        value: player.position
                        enabled: window.hasCurrentAudio()
                        onMoved: player.position = value
                    }
                    Label {
                        text: window.formatTime(player.position) + " / " + window.formatTime(player.duration)
                        color: window.mutedText
                        font.pixelSize: 11
                    }
                }
            }
        }
    }

    function current() {
        return utterances.get(selectedIndex);
    }

    function qtShortcutSequence(sequence) {
        const parts = String(sequence || "").split("+");
        if (parts.length && parts[parts.length - 1] === "Enter")
            parts[parts.length - 1] = "Return";
        return parts.join("+");
    }

    function showVoicebankDetails() {
        if (!window.appBackend.voicebanks.length)
            return;
        voicebankDetailsWindow.currentIndex = Math.max(0, Math.min(voicebankDetailsWindow.currentIndex, window.appBackend.voicebanks.length - 1));
        window.showAuxiliaryWindow(voicebankDetailsWindow);
    }

    function saveSettings() {
        const shortcuts = [settingsWindow.pendingSynthesizeShortcut,
                           settingsWindow.pendingSaveProjectShortcut,
                           settingsWindow.pendingReloadVoicebanksShortcut,
                           settingsWindow.pendingAddUtteranceShortcut,
                           settingsWindow.pendingRemoveUtteranceShortcut];
        const usedShortcuts = [];
        for (let index = 0; index < shortcuts.length; ++index) {
            const shortcut = String(shortcuts[index] || "").trim();
            if (!shortcut.length)
                continue;
            const normalized = window.qtShortcutSequence(shortcut).toLowerCase();
            if (usedShortcuts.indexOf(normalized) >= 0) {
                shortcutConflictDialog.open();
                return;
            }
            usedShortcuts.push(normalized);
        }
        if (utterances.count) {
            window.updateSetting("moraDuration", settingsWindow.pendingMoraDuration);
            window.updateSetting("pauseDuration", settingsWindow.pendingPauseDuration);
            window.updateSetting("applyPitch", settingsWindow.pendingApplyPitch);
            window.selectUtterance(window.selectedIndex);
        }
        window.appBackend.setSynthesisDefaults(settingsWindow.pendingMoraDuration, settingsWindow.pendingPauseDuration, settingsWindow.pendingApplyPitch);
        window.appBackend.setDarkMode(settingsWindow.pendingDarkMode);
        window.appBackend.setCloseLogOnSuccess(settingsWindow.pendingCloseLogOnSuccess);
        window.appBackend.setShortcutSequences(settingsWindow.pendingSynthesizeShortcut,
                                               settingsWindow.pendingSaveProjectShortcut,
                                               settingsWindow.pendingReloadVoicebanksShortcut,
                                               settingsWindow.pendingAddUtteranceShortcut,
                                               settingsWindow.pendingRemoveUtteranceShortcut);
        settingsWindow.close();
        settingsWindow.visible = false;
    }

    function showAuxiliaryWindow(auxiliaryWindow) {
        auxiliaryWindow.visible = true;
        auxiliaryWindow.raise();
        auxiliaryWindow.requestActivate();
    }

    function openSettings() {
        settingsWindow.loadCurrent();
        showAuxiliaryWindow(settingsWindow);
    }

    function openDictionarySettings() {
        dictionaryWindow.loadCurrent();
        showAuxiliaryWindow(dictionaryWindow);
    }

    function voicebankById(id) {
        for (let i = 0; i < window.appBackend.voicebanks.length; ++i)
            if (window.appBackend.voicebanks[i].id === id)
                return window.appBackend.voicebanks[i];
        return null;
    }

    function modelById(id) {
        for (let i = 0; i < window.appBackend.models.length; ++i)
            if (window.appBackend.models[i].id === id)
                return window.appBackend.models[i];
        return null;
    }

    function rendererById(id) {
        for (let i = 0; i < window.appBackend.renderers.length; ++i)
            if (window.appBackend.renderers[i].id === id)
                return window.appBackend.renderers[i];
        return null;
    }

    function modelDescription(id) {
        if (!id || id === "none")
            return "モデルを使わず、原音のピッチを維持します。";
        const model = modelById(id);
        return model ? model.description || "" : "";
    }

    function rendererDescription(id) {
        if (!id)
            return "既定Rendererはplugin manifestの優先度から選択されます。";
        const renderer = rendererById(id);
        return renderer ? renderer.description || "" : "";
    }

    function defaultModelId() {
        return window.appBackend.models.length ? window.appBackend.models[0].id : "none";
    }

    function preferredRendererForModel(model) {
        const preferredAcceleration = window.appBackend.cudaAvailable ? "cuda" : "cpu";
        const recommended = model && model.recommended_renderers ? model.recommended_renderers : [];
        for (let pass = 0; pass < 2; ++pass) {
            for (let index = 0; index < recommended.length; ++index) {
                const renderer = window.rendererById(recommended[index]);
                if (renderer && (pass === 1 || renderer.acceleration === preferredAcceleration))
                    return renderer.id;
            }
        }
        for (let index = 0; index < window.appBackend.renderers.length; ++index) {
            const renderer = window.appBackend.renderers[index];
            if (renderer.acceleration === preferredAcceleration)
                return renderer.id;
        }
        return window.appBackend.defaultRenderer;
    }

    function defaultRendererId() {
        return window.preferredRendererForModel(window.modelById(window.defaultModelId()));
    }

    function normalizeRendererId(id) {
        const rendererId = String(id || "");
        if (rendererId && window.rendererById(rendererId))
            return rendererId;
        const waveform = window.rendererById("waveform");
        return waveform ? waveform.id : window.defaultRendererId();
    }

    function utteranceIndex(id) {
        for (let i = 0; i < utterances.count; ++i)
            if (utterances.get(i).utteranceId === id)
                return i;
        return -1;
    }

    function voicebankName(id) {
        const voice = voicebankById(id);
        return voice ? voice.name : "音源未選択";
    }

    function fileNamePart(value, fallback) {
        let result = String(value === undefined || value === null ? "" : value)
                .replace(/[<>:"\/\\|?*\x00-\x1F]/g, " ")
                .replace(/\s+/g, " ")
                .trim();
        while (result.endsWith(".") || result.endsWith(" "))
            result = result.slice(0, -1).trim();
        return result || fallback;
    }

    function audioFileName(item) {
        const voice = fileNamePart(window.voicebankName(item.voicebankId), "voicebank");
        const text = fileNamePart(item.content, "utterance-" + item.utteranceId);
        return voice + "_" + text + ".wav";
    }

    function dragAudioFileName(item, index) {
        const number = ("000" + String(index + 1)).slice(-3);
        return number + "_" + window.audioFileName(item);
    }

    function saveCurrentAudio() {
        if (!utterances.count || window.appBackend.busy || window.batchExportActive || !window.current().content.trim().length)
            return;
        const item = window.current();
        window.clearPlayback();
        window.saveRequestPending = true;
        window.pendingUtteranceId = item.utteranceId;
        window.pendingRevision = item.revision;
        window.appBackend.clearLogs();
        window.showAuxiliaryWindow(synthesisLogWindow);
        window.appBackend.synthesize(window.buildSynthesisRequest(item));
    }

    function openSaveAllDialog() {
        if (!utterances.count || window.appBackend.busy || window.batchExportActive)
            return;
        saveAllDialog.open();
    }

    function openDragExportDialog(selectedOnly) {
        if (!utterances.count || window.appBackend.busy || window.batchExportActive)
            return;
        if (selectedOnly && !window.current().content.trim().length)
            return;
        window.dragExportSelectedOnly = selectedOnly;
        dragSaveDialog.open();
    }

    function openDragTargetSelectionWindow(directory) {
        if (!directory || !directory.toString().length)
            return;
        window.batchExportDirectory = directory;
        window.dragExportReady = false;
        window.dragExportFiles = [];
        dragTargetSelectionWindow.reset();
        window.showAuxiliaryWindow(dragTargetSelectionWindow);
    }

    function projectNumber(value, fallback, minimum, maximum, integer) {
        const parsed = Number(value);
        if (!Number.isFinite(parsed))
            return fallback;
        const normalized = integer ? Math.round(parsed) : parsed;
        return Math.max(minimum, Math.min(maximum, normalized));
    }

    function projectData() {
        const savedUtterances = [];
        for (let index = 0; index < utterances.count; ++index) {
            const item = utterances.get(index);
            savedUtterances.push({
                text: item.content || "",
                voicebank_id: item.voicebankId || "",
                model_id: item.modelId || "",
                renderer_id: item.renderer || "",
                tone: item.tone || "C4",
                mora_duration_ms: item.moraDuration,
                pause_duration_ms: item.pauseDuration,
                intonation: item.intonation,
                apply_pitch: !!item.applyPitch,
                pitch_points: window.decodeSequence(item.pointsJson),
                mora_durations_ms: window.decodeSequence(item.moraDurationsJson),
                mora_positions_ms: window.decodeSequence(item.moraPositionsJson),
                automatic_pitch_points: window.automaticSequence(item, "autoPointsJson"),
                automatic_mora_durations_ms: window.automaticSequence(item, "autoMoraDurationsJson"),
                automatic_mora_positions_ms: window.automaticSequence(item, "autoMoraPositionsJson"),
                manual_pitch_edited: window.hasManualPitch(item),
                manual_mora_duration_edited: window.hasManualMoraDurations(item),
                analysis_cache: {
                    reading: item.reading || "",
                    morae: window.decodeSequence(item.moraeJson)
                }
            });
        }
        return {
            format: "utautts-project",
            format_version: 1,
            app_version: Qt.application.version,
            utterances: savedUtterances,
            selected_index: utterances.count ? selectedIndex : 0
        };
    }

    function reanalyzeAll() {
        window.clearPlayback();
        for (let index = 0; index < utterances.count; ++index) {
            const item = utterances.get(index);
            utterances.setProperty(index, "reading", "");
            utterances.setProperty(index, "moraeJson", "[]");
            if (item.content.trim())
                window.appBackend.analyze(item.content, item.utteranceId);
        }
        if (utterances.count)
            window.selectUtterance(window.selectedIndex);
    }

    function openProjectSaveDialog() {
        if (window.appBackend.busy || window.batchExportActive)
            return;
        projectSaveDialog.currentFile = window.appBackend.defaultSaveFile("untitled.utautts");
        projectSaveDialog.open();
    }

    function saveProjectTo(destination) {
        if (!destination || !destination.toString().length)
            return;
        const saved = window.appBackend.saveProject(destination, window.projectData());
        if (!saved) {
            window.closeAfterProjectSave = false;
            return;
        }
        window.projectDirty = false;
        if (window.closeAfterProjectSave) {
            window.closeAfterProjectSave = false;
            window.quitWithoutWarning();
        }
    }

    function loadProjectFrom(source) {
        if (!source || !source.toString().length || window.appBackend.busy || window.batchExportActive)
            return;
        const project = window.appBackend.loadProject(source);
        if (!project || project._error !== undefined) {
            projectLoadErrorDialog.text = project && project._error !== undefined
                    ? String(project._error) : "プロジェクトファイルを読み込めませんでした";
            projectLoadErrorDialog.open();
            return;
        }
        if (project.utterances === undefined || project.utterances === null) {
            projectLoadErrorDialog.text = "プロジェクトファイルに発話データがありません";
            projectLoadErrorDialog.open();
            return;
        }
        const loadedUtterances = window.copySequence(project.utterances);

        window.projectDirty = false;
        window.clearPlayback();
        utterances.clear();
        window.nextUtteranceId = 1;
        let migratedRenderer = false;
        for (let index = 0; index < loadedUtterances.length; ++index) {
            const saved = loadedUtterances[index] || {};
            const voicebankId = String(saved.voicebank_id || "");
            const voice = window.voicebankById(voicebankId);
            const points = window.copySequence(saved.pitch_points);
            const content = String(saved.text || "");
            const rendererId = window.normalizeRendererId(saved.renderer_id);
            if (String(saved.renderer_id || "") !== rendererId)
                migratedRenderer = true;
            utterances.append({
                utteranceId: "utterance-" + window.nextUtteranceId++,
                content: content,
                reading: "",
                moraeJson: "[]",
                pointsJson: JSON.stringify(points),
                moraDurationsJson: JSON.stringify(window.copySequence(saved.mora_durations_ms)),
                moraPositionsJson: JSON.stringify(window.copySequence(saved.mora_positions_ms)),
                autoPointsJson: JSON.stringify(window.copySequence(saved.automatic_pitch_points)),
                autoMoraDurationsJson: JSON.stringify(window.copySequence(saved.automatic_mora_durations_ms)),
                autoMoraPositionsJson: JSON.stringify(window.copySequence(saved.automatic_mora_positions_ms)),
                manualPitchEdited: saved.manual_pitch_edited === undefined
                        ? points.some(value => Math.abs(Number(value)) > .1) : !!saved.manual_pitch_edited,
                manualMoraDurationEdited: saved.manual_mora_duration_edited === undefined
                        ? window.copySequence(saved.mora_durations_ms).some(value => Number(value) > 0)
                        : !!saved.manual_mora_duration_edited,
                voicebankId: voicebankId,
                imagePath: voice ? voice.image_path || "" : "",
                modelId: String(saved.model_id || ""),
                renderer: rendererId,
                tone: String(saved.tone || "C4"),
                moraDuration: window.projectNumber(saved.mora_duration_ms, window.appBackend.defaultMoraDuration, 20, 1000, true),
                pauseDuration: window.projectNumber(saved.pause_duration_ms, window.appBackend.defaultPauseDuration, 0, 3000, true),
                intonation: window.projectNumber(saved.intonation, 0, 0, 2, false),
                applyPitch: saved.apply_pitch === undefined ? window.appBackend.defaultApplyPitch : !!saved.apply_pitch,
                revision: 0
            });
        }

        window.projectDirty = migratedRenderer;

        if (!utterances.count) {
            selectedIndex = 0;
            pitchEditor.points = [];
            pitchEditor.morae = [];
            pitchEditor.moraDurations = [];
            pitchEditor.moraPositions = [];
            return;
        }
        selectedIndex = Math.max(0, Math.min(Number(project.selected_index) || 0, utterances.count - 1));
        window.selectUtterance(selectedIndex);
        for (let index = 0; index < utterances.count; ++index) {
            const item = utterances.get(index);
            if (item.content.trim())
                window.appBackend.analyze(item.content, item.utteranceId);
        }
        utteranceList.positionViewAtIndex(selectedIndex, ListView.Contain);
    }

    function localImageUrl(path) {
        return path ? encodeURI("file:///" + path.replace(/\\/g, "/")) : "";
    }

    function formatTime(milliseconds) {
        const seconds = Math.max(0, Math.floor(milliseconds / 1000));
        return Math.floor(seconds / 60) + ":" + String(seconds % 60).padStart(2, "0");
    }

    function updateSetting(name, value) {
        if (!utterances.count)
            return;
        const item = current();
        if (item[name] === value)
            return;
        utterances.setProperty(selectedIndex, name, value);
        if (["voicebankId", "modelId", "renderer", "tone", "moraDuration", "pauseDuration",
             "intonation", "applyPitch"].indexOf(name) >= 0)
            clearAutomaticProsody(selectedIndex);
        if (name === "moraDuration")
            pitchEditor.defaultMoraDuration = value;
        else if (name === "pauseDuration")
            pitchEditor.defaultPauseDuration = value;
        markUtteranceDirty(selectedIndex);
        if (["modelId", "renderer", "moraDuration", "pauseDuration", "intonation", "applyPitch"].indexOf(name) >= 0) {
            const updated = utterances.get(selectedIndex);
            if (updated.content.trim() && updated.reading)
                Qt.callLater(function() { window.requestProsodyPreview(selectedIndex); });
        }
    }

    function updateUtteranceText(index, text) {
        if (index < 0 || index >= utterances.count)
            return;
        utterances.setProperty(index, "content", text);
        utterances.setProperty(index, "reading", "");
        utterances.setProperty(index, "moraeJson", "[]");
        utterances.setProperty(index, "pointsJson", "[]");
        utterances.setProperty(index, "moraDurationsJson", "[]");
        utterances.setProperty(index, "moraPositionsJson", "[]");
        utterances.setProperty(index, "autoPointsJson", "[]");
        utterances.setProperty(index, "autoMoraDurationsJson", "[]");
        utterances.setProperty(index, "autoMoraPositionsJson", "[]");
        utterances.setProperty(index, "manualPitchEdited", false);
        utterances.setProperty(index, "manualMoraDurationEdited", false);
        markUtteranceDirty(index);
        selectUtterance(index);
        analyzeTimer.restart();
    }

    function updatePitchPoints(points) {
        if (!utterances.count)
            return;
        utterances.setProperty(selectedIndex, "pointsJson", JSON.stringify(points));
        utterances.setProperty(selectedIndex, "manualPitchEdited", true);
        if (!current().applyPitch) {
            utterances.setProperty(selectedIndex, "applyPitch", true);
            settingsWindow.pendingApplyPitch = true;
        }
        markUtteranceDirty(selectedIndex);
    }

    function updateMoraDurations(durations) {
        if (!utterances.count)
            return;
        utterances.setProperty(selectedIndex, "moraDurationsJson", JSON.stringify(durations));
        utterances.setProperty(selectedIndex, "manualMoraDurationEdited", true);
        markUtteranceDirty(selectedIndex);
    }

    function updateMoraPositions(positions) {
        if (!utterances.count)
            return;
        utterances.setProperty(selectedIndex, "moraPositionsJson", JSON.stringify(positions));
        utterances.setProperty(selectedIndex, "manualMoraDurationEdited", true);
        markUtteranceDirty(selectedIndex);
    }

    function markUtteranceDirty(index, markProject) {
        if (index < 0 || index >= utterances.count)
            return;
        const item = utterances.get(index);
        utterances.setProperty(index, "revision", item.revision + 1);
        if (markProject !== false)
            window.projectDirty = true;
        if (window.audioUtteranceId === item.utteranceId)
            clearPlayback();
    }

    function hasCurrentAudio() {
        if (!utterances.count || !window.audioUtteranceId || !player.source.toString().length)
            return false;
        const item = current();
        return item.utteranceId === window.audioUtteranceId && item.revision === window.audioRevision;
    }

    function hasCachedAudio() {
        if (!window.audioUtteranceId || !player.source.toString().length)
            return false;
        const index = window.utteranceIndex(window.audioUtteranceId);
        return index >= 0 && utterances.get(index).revision === window.audioRevision;
    }

    function stopPlaybackQueue() {
        window.playbackQueueActive = false;
        window.playbackQueue = [];
        window.playbackQueueIndex = -1;
        window.pendingUtteranceId = "";
        window.pendingRevision = -1;
    }

    function replayCachedAudio() {
        if (!window.hasCachedAudio())
            return;
        window.stopPlaybackQueue();
        window.playbackRequested = false;
        window.playbackError = "";
        player.stop();
        if (player.duration > 0)
            player.position = 0;
        player.play();
    }

    function clearPlayback(stopQueue) {
        if (stopQueue !== false)
            window.stopPlaybackQueue();
        window.playbackRequested = false;
        window.playbackError = "";
        player.stop();
        player.source = "";
        window.audioUtteranceId = "";
        window.audioRevision = -1;
    }

    function assignDefaultVoicebank(suppressDirty) {
        if (!utterances.count || !window.appBackend.voicebanks.length)
            return;
        for (let i = 0; i < utterances.count; ++i) {
            const item = utterances.get(i);
            if (!item.voicebankId || !window.voicebankById(item.voicebankId)) {
                utterances.setProperty(i, "voicebankId", window.appBackend.voicebanks[0].id);
                utterances.setProperty(i, "imagePath", window.appBackend.voicebanks[0].image_path || "");
                markUtteranceDirty(i, suppressDirty !== true);
            }
        }
        selectUtterance(selectedIndex);
    }

    function assignDefaultSynthesisSettings(suppressDirty) {
        if (!utterances.count || !window.appBackend.models.length || !window.appBackend.renderers.length)
            return;
        const modelId = window.defaultModelId();
        const rendererId = window.defaultRendererId();
        for (let index = 0; index < utterances.count; ++index) {
            const item = utterances.get(index);
            let changed = false;
            if (!item.modelId) {
                utterances.setProperty(index, "modelId", modelId);
                changed = true;
            }
            if (!item.renderer) {
                utterances.setProperty(index, "renderer", rendererId);
                changed = true;
            }
            if (changed)
                markUtteranceDirty(index, suppressDirty !== true);
        }
        selectUtterance(selectedIndex);
    }

    function selectCombo(combo, value) {
        for (let i = 0; i < combo.count; ++i) {
            if (combo.valueAt(i) === value) {
                combo.currentIndex = i;
                return;
            }
        }
    }

    function selectUtterance(index, preservePlaybackQueue) {
        if (index < 0 || index >= utterances.count)
            return;
        const changed = index !== selectedIndex;
        if (changed) {
            if (preservePlaybackQueue === true)
                clearPlayback(false);
            else
                clearPlayback();
        }
        selectedIndex = index;
        const item = current();
        toneField.text = item.tone;
        moraSlider.value = item.moraDuration;
        pauseSlider.value = item.pauseDuration;
        moraInput.value = item.moraDuration;
        pauseInput.value = item.pauseDuration;
        intonationSlider.value = item.intonation;
        intonationInput.value = Math.round(item.intonation * 100);
        pitchEditor.points = window.decodeSequence(item.pointsJson);
        pitchEditor.autoPoints = window.automaticSequence(item, "autoPointsJson");
        pitchEditor.morae = window.decodeSequence(item.moraeJson);
        pitchEditor.defaultMoraDuration = item.moraDuration;
        pitchEditor.defaultPauseDuration = item.pauseDuration;
        pitchEditor.moraDurations = window.displayedMoraDurations(item);
        pitchEditor.moraPositions = window.displayedMoraPositions(item);
        selectCombo(voiceCombo, item.voicebankId);
        selectCombo(modelCombo, item.modelId);
        selectCombo(rendererCombo, item.renderer);
    }

    function copySequence(sequence) {
        const result = [];
        if (!sequence)
            return result;
        const size = sequence.length !== undefined ? sequence.length : sequence.count;
        for (let index = 0; index < size; ++index)
            result.push(sequence.get ? sequence.get(index) : sequence[index]);
        return result;
    }

    function decodeSequence(json) {
        if (!json || !json.length)
            return [];
        try {
            const value = JSON.parse(json);
            return Array.isArray(value) ? value : [];
        } catch (error) {
            return [];
        }
    }

    function hasManualPitch(item) {
        if (item && item.manualPitchEdited)
            return true;
        return decodeSequence(item ? item.pointsJson : "").some(value => Math.abs(Number(value)) > .1);
    }

    function hasManualMoraDurations(item) {
        if (item && item.manualMoraDurationEdited)
            return true;
        return decodeSequence(item ? item.moraDurationsJson : "").some(value => Number(value) > 0);
    }

    function automaticSequence(item, name) {
        return decodeSequence(item ? item[name] : "[]");
    }

    function displayedMoraDurations(item) {
        if (hasManualMoraDurations(item))
            return decodeSequence(item.moraDurationsJson);
        return automaticSequence(item, "autoMoraDurationsJson");
    }

    function displayedMoraPositions(item) {
        if (hasManualMoraDurations(item))
            return decodeSequence(item.moraPositionsJson);
        return automaticSequence(item, "autoMoraPositionsJson");
    }

    function clearAutomaticProsody(index) {
        if (index < 0 || index >= utterances.count)
            return;
        utterances.setProperty(index, "autoPointsJson", "[]");
        utterances.setProperty(index, "autoMoraDurationsJson", "[]");
        utterances.setProperty(index, "autoMoraPositionsJson", "[]");
        if (index === selectedIndex) {
            const item = utterances.get(index);
            pitchEditor.autoPoints = [];
            pitchEditor.moraDurations = hasManualMoraDurations(item)
                    ? decodeSequence(item.moraDurationsJson) : [];
            pitchEditor.moraPositions = hasManualMoraDurations(item)
                    ? decodeSequence(item.moraPositionsJson) : [];
        }
    }

    function resetMoraDuration() {
        moraSlider.value = 120;
        moraInput.value = 120;
        window.updateSetting("moraDuration", 120);
    }

    function resetIntonation() {
        intonationSlider.value = 0;
        intonationInput.value = 0;
        window.updateSetting("intonation", 0);
    }

    function resetPauseDuration() {
        pauseSlider.value = 180;
        pauseInput.value = 180;
        window.updateSetting("pauseDuration", 180);
    }

    function addUtterance(markDirty) {
        const voice = window.appBackend.voicebanks.length ? window.appBackend.voicebanks[0] : null;
        utterances.append({
            utteranceId: "utterance-" + nextUtteranceId++,
            content: "",
            reading: "",
            moraeJson: "[]",
            pointsJson: "[]",
            moraDurationsJson: "[]",
            moraPositionsJson: "[]",
            autoPointsJson: "[]",
            autoMoraDurationsJson: "[]",
            autoMoraPositionsJson: "[]",
            manualPitchEdited: false,
            manualMoraDurationEdited: false,
            voicebankId: voice ? voice.id : "",
            imagePath: voice ? voice.image_path || "" : "",
            modelId: window.appBackend.models.length ? window.defaultModelId() : "",
            renderer: window.appBackend.renderers.length ? window.defaultRendererId() : "",
            tone: "C4",
            moraDuration: window.appBackend.defaultMoraDuration,
            pauseDuration: window.appBackend.defaultPauseDuration,
            intonation: 0,
            applyPitch: window.appBackend.defaultApplyPitch,
            revision: 0
        });
        if (markDirty !== false)
            window.projectDirty = true;
        const newIndex = utterances.count - 1;
        selectUtterance(newIndex);
        utteranceList.positionViewAtEnd();
        Qt.callLater(() => {
            const newCard = utteranceList.itemAtIndex(newIndex);
            if (!newCard || !newCard.textEditor)
                return;
            newCard.textEditor.forceActiveFocus();
            newCard.textEditor.selectAll();
        });
    }

    function removeUtterance() {
        clearPlayback();
        utterances.remove(selectedIndex);
        window.projectDirty = true;
        if (!utterances.count) {
            selectedIndex = 0;
            pitchEditor.points = [];
            pitchEditor.morae = [];
            return;
        }
        selectedIndex = Math.min(selectedIndex, utterances.count - 1);
        selectUtterance(selectedIndex);
    }

    function moveUtterance(delta) {
        const target = selectedIndex + delta;
        if (target < 0 || target >= utterances.count)
            return;
        window.clearPlayback();
        utterances.move(selectedIndex, target, 1);
        window.projectDirty = true;
        selectedIndex = target;
        utteranceList.positionViewAtIndex(target, ListView.Contain);
    }

    function hasPlayableTextFrom(startIndex) {
        const first = Math.max(0, Number(startIndex) || 0);
        for (let index = first; index < utterances.count; ++index) {
            if (utterances.get(index).content.trim().length)
                return true;
        }
        return false;
    }

    function startPlaybackQueue(startIndex) {
        if (window.appBackend.busy || window.batchExportActive || window.playbackQueueActive)
            return;
        const first = Math.max(0, Number(startIndex) || 0);
        const queue = [];
        for (let index = first; index < utterances.count; ++index) {
            if (utterances.get(index).content.trim().length)
                queue.push(index);
        }
        if (!queue.length)
            return;

        window.stopPlaybackQueue();
        window.clearPlayback(false);
        window.playbackQueue = queue;
        window.playbackQueueIndex = 0;
        window.playbackQueueActive = true;
        window.appBackend.clearLogs();
        window.showAuxiliaryWindow(synthesisLogWindow);
        window.playNextPlaybackItem();
    }

    function playNextPlaybackItem() {
        if (!window.playbackQueueActive)
            return;
        if (window.playbackQueueIndex >= window.playbackQueue.length) {
            window.finishPlaybackQueue();
            return;
        }

        const index = Number(window.playbackQueue[window.playbackQueueIndex]);
        if (index < 0 || index >= utterances.count || !utterances.get(index).content.trim().length) {
            ++window.playbackQueueIndex;
            Qt.callLater(window.playNextPlaybackItem);
            return;
        }

        window.selectUtterance(index, true);
        window.clearPlayback(false);
        const item = utterances.get(index);
        window.pendingUtteranceId = item.utteranceId;
        window.pendingRevision = item.revision;
        window.appBackend.synthesize(window.buildSynthesisRequest(item));
    }

    function finishPlaybackQueue() {
        const closeLog = window.appBackend.closeLogOnSuccess;
        window.stopPlaybackQueue();
        if (closeLog)
            synthesisLogWindow.close();
    }

    function synthesizeCurrent() {
        const item = current();
        clearPlayback();
        window.pendingUtteranceId = item.utteranceId;
        window.pendingRevision = item.revision;
        window.appBackend.clearLogs();
        window.showAuxiliaryWindow(synthesisLogWindow);
        window.appBackend.synthesize(window.buildSynthesisRequest(item));
    }

    function quitWithoutWarning() {
        window.closeBypass = true;
        Qt.quit();
    }

    function buildSynthesisRequest(item) {
        const points = window.decodeSequence(item.pointsJson);
        const morae = window.decodeSequence(item.moraeJson);
        const manualPitch = window.hasManualPitch(item);
        const manualDurations = window.hasManualMoraDurations(item)
                ? window.decodeSequence(item.moraDurationsJson) : [];
        const request = {
            text: item.content,
            kana: item.reading || "",
            dictionary: window.appBackend.dictionaryEntries,
            voicebank_id: item.voicebankId || voiceCombo.currentValue,
            model_id: item.modelId,
            renderer: item.renderer,
            tone: item.tone,
            mora_duration_ms: item.moraDuration,
            pause_duration_ms: item.pauseDuration,
            mora_durations_ms: manualDurations,
            intonation_strength: item.intonation,
            apply_pitch: item.applyPitch
        };
        if (item.applyPitch && item.reading && manualPitch && points.some(value => Math.abs(Number(value)) > .1)) {
            const manualPoints = [];
            for (let index = 0; index < points.length; ++index) {
                const mora = index < morae.length ? morae[index] : null;
                if (mora && mora.pause)
                    continue;
                manualPoints.push({
                    position: index,
                    mora: mora ? mora.mora || "" : "",
                    cents: points[index]
                });
            }
            request.manual_pitch = {
                version: 1,
                reading: item.reading,
                mode: "offset",
                points: manualPoints
            };
        }
        return request;
    }

    function buildProsodyRequest(item, requestId) {
        return {
            request_id: requestId,
            text: item.content,
            kana: item.reading || "",
            dictionary: window.appBackend.dictionaryEntries,
            model_id: item.modelId,
            renderer: item.renderer,
            mora_duration_ms: item.moraDuration,
            pause_duration_ms: item.pauseDuration,
            mora_durations_ms: window.hasManualMoraDurations(item)
                    ? window.decodeSequence(item.moraDurationsJson) : [],
            intonation_strength: item.intonation,
            apply_pitch: item.applyPitch
        };
    }

    function requestProsodyPreview(index) {
        if (index < 0 || index >= utterances.count)
            return;
        const item = utterances.get(index);
        if (!item.content.trim() || !item.reading)
            return;
        const requestId = item.utteranceId + ":" + item.revision + ":" + Date.now();
        window.pendingProsodyRequestId = requestId;
        window.pendingProsodyUtteranceId = item.utteranceId;
        window.pendingProsodyRevision = item.revision;
        window.appBackend.predictProsody(window.buildProsodyRequest(item, requestId));
    }

    function buildExportQueue(selectedOnly) {
        const queue = [];
        if (selectedOnly) {
            if (utterances.count && window.current().content.trim().length)
                queue.push(window.selectedIndex);
            return queue;
        }
        for (let index = 0; index < utterances.count; ++index) {
            if (utterances.get(index).content && utterances.get(index).content.trim().length)
                queue.push(index);
        }
        return queue;
    }

    function beginBatchExport(directory, mode, queue) {
        if (!directory || !directory.toString().length || !queue.length || window.appBackend.busy)
            return;
        window.batchExportDirectory = directory;
        window.batchExportMode = mode;
        window.batchExportQueue = queue;
        window.batchExportOriginalIndex = window.selectedIndex;
        window.batchExportIndex = 0;
        window.batchExportCompleted = 0;
        window.dragExportFiles = [];
        window.batchExportActive = true;
        window.clearPlayback();
        window.pendingUtteranceId = "";
        window.pendingRevision = -1;
        window.appBackend.clearLogs();
        window.showAuxiliaryWindow(synthesisLogWindow);
        Qt.callLater(window.synthesizeBatchItem);
    }

    function startBatchExport(directory) {
        window.beginBatchExport(directory, "save", window.buildExportQueue(false));
    }

    function startDragExport(target) {
        const queue = window.buildExportQueue(window.dragExportSelectedOnly);
        if (!target || !queue.length)
            return;
        window.dragExportTarget = target;
        window.dragExportReady = false;
        window.beginBatchExport(window.batchExportDirectory, "drag", queue);
    }

    function synthesizeBatchItem() {
        if (!window.batchExportActive)
            return;
        while (window.batchExportIndex < window.batchExportQueue.length) {
            const index = window.batchExportQueue[window.batchExportIndex++];
            const item = utterances.get(index);
            window.selectUtterance(index);
            const requestItem = utterances.get(index);
            window.pendingUtteranceId = requestItem.utteranceId;
            window.pendingRevision = requestItem.revision;
            window.appBackend.synthesize(window.buildSynthesisRequest(requestItem));
            return;
        }
        window.finishBatchExport(true);
    }

    function finishBatchExport(success) {
        if (!window.batchExportActive)
            return;
        const wasDragExport = window.batchExportMode === "drag";
        const dragExportSucceeded = success && wasDragExport;
        const files = window.dragExportFiles.slice();
        window.batchExportActive = false;
        window.batchExportMode = "";
        window.batchExportQueue = [];
        window.pendingUtteranceId = "";
        window.pendingRevision = -1;
        if (utterances.count)
            window.selectUtterance(Math.min(window.batchExportOriginalIndex, utterances.count - 1));
        if (success && (window.appBackend.closeLogOnSuccess || wasDragExport))
            synthesisLogWindow.close();
        if (dragExportSucceeded && files.length) {
            window.dragExportFiles = files;
            window.dragExportReady = true;
            window.showAuxiliaryWindow(dragTargetWindow);
        } else if (!success && wasDragExport) {
            window.dragExportReady = false;
        }
    }
}
