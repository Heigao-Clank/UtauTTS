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
        highlight: window.darkMode ? "#4fae79" : "#2f8b5b"
        highlightedText: "#ffffff"
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
    property bool playbackRequested: false
    property string playbackError: ""

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

    ApplicationWindow {
        id: synthesisLogWindow
        title: "音声合成ログ"
        visible: false
        width: 720
        height: 420
        minimumWidth: 520
        minimumHeight: 280
        transientParent: window
        palette: window.palette
        color: palette.window

        ColumnLayout {
            anchors.fill: parent
            anchors.margins: 12
            spacing: 8

            Label {
                Layout.fillWidth: true
                text: window.appBackend.busy ? "音声合成を実行中です…" : "音声合成ログ"
                font.bold: true
            }

            ScrollView {
                Layout.fillWidth: true
                Layout.fillHeight: true
                TextArea {
                    id: synthesisLogText
                    width: synthesisLogWindow.width - 36
                    text: window.appBackend.logLines.join("\n")
                    readOnly: true
                    selectByMouse: true
                    wrapMode: TextEdit.Wrap
                    onTextChanged: cursorPosition = length
                }
            }

            RowLayout {
                Layout.fillWidth: true
                Item {
                    Layout.fillWidth: true
                }
                Button {
                    text: "閉じる"
                    onClicked: synthesisLogWindow.close()
                }
            }
        }
    }

    ApplicationWindow {
        id: settingsWindow
        title: "設定"
        visible: false
        width: 720
        height: 540
        minimumWidth: 580
        minimumHeight: 420
        transientParent: window
        modality: Qt.ApplicationModal
        flags: Qt.Dialog
        palette: window.palette
        color: palette.window
        property int currentPage: 0
        property int pendingMoraDuration: 120
        property int pendingPauseDuration: 180
        property bool pendingApplyPitch: true
        property bool pendingDarkMode: false
        property bool pendingCloseLogOnSuccess: true

        function loadCurrent() {
            pendingMoraDuration = window.appBackend.defaultMoraDuration;
            pendingPauseDuration = window.appBackend.defaultPauseDuration;
            pendingApplyPitch = window.appBackend.defaultApplyPitch;
            pendingDarkMode = window.appBackend.darkMode;
            pendingCloseLogOnSuccess = window.appBackend.closeLogOnSuccess;
        }

        RowLayout {
            anchors.fill: parent
            anchors.margins: 12
            spacing: 12

            ListView {
                id: settingsNavigation
                Layout.preferredWidth: 170
                Layout.fillHeight: true
                clip: true
                model: ["タイミング", "ピッチ加工", "外観", "ログ"]
                currentIndex: settingsWindow.currentPage

                delegate: ItemDelegate {
                    required property int index
                    required property string modelData
                    width: ListView.view.width
                    text: modelData
                    highlighted: ListView.isCurrentItem
                    onClicked: settingsWindow.currentPage = index
                }
            }

            Rectangle {
                Layout.preferredWidth: 1
                Layout.fillHeight: true
                color: window.borderColor
            }

            ColumnLayout {
                Layout.fillWidth: true
                Layout.fillHeight: true
                spacing: 10

                StackLayout {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    currentIndex: settingsWindow.currentPage

                    ScrollView {
                        id: timingSettingsPage
                        contentWidth: availableWidth

                        ColumnLayout {
                            width: timingSettingsPage.availableWidth
                            spacing: 14

                            Label {
                                text: "タイミング"
                                font.pixelSize: 18
                                font.bold: true
                            }
                            Label {
                                Layout.fillWidth: true
                                text: "発音と休止の標準時間を設定します。各セリフの右ペインから個別に上書きできます。"
                                color: window.mutedText
                                wrapMode: Text.Wrap
                            }
                            GridLayout {
                                Layout.fillWidth: true
                                columns: 2
                                columnSpacing: 12
                                rowSpacing: 10

                                Label {
                                    text: "モーラ長"
                                }
                                SpinBox {
                                    id: moraSpin
                                    Layout.fillWidth: true
                                    from: 20
                                    to: 1000
                                    value: settingsWindow.pendingMoraDuration
                                    editable: true
                                    textFromValue: value => value + " ms"
                                    valueFromText: text => parseInt(text)
                                    onValueModified: settingsWindow.pendingMoraDuration = value
                                    TapHandler {
                                        acceptedButtons: Qt.LeftButton
                                        grabPermissions: PointerHandler.CanTakeOverFromAnything
                                        onDoubleTapped: {
                                            settingsWindow.pendingMoraDuration = 120;
                                        }
                                    }
                                }
                                Label {
                                    text: "休止長"
                                }
                                SpinBox {
                                    id: pauseSpin
                                    Layout.fillWidth: true
                                    from: 0
                                    to: 3000
                                    value: settingsWindow.pendingPauseDuration
                                    editable: true
                                    textFromValue: value => value + " ms"
                                    valueFromText: text => parseInt(text)
                                    onValueModified: settingsWindow.pendingPauseDuration = value
                                    TapHandler {
                                        acceptedButtons: Qt.LeftButton
                                        grabPermissions: PointerHandler.CanTakeOverFromAnything
                                        onDoubleTapped: {
                                            settingsWindow.pendingPauseDuration = 180;
                                        }
                                    }
                                }
                            }
                            Label {
                                Layout.fillWidth: true
                                text: "モーラ長は通常の発音単位、休止長は句読点で挿入する無音の基準時間です。"
                                color: window.mutedText
                                wrapMode: Text.Wrap
                            }
                        }
                    }

                    ScrollView {
                        id: pitchSettingsPage
                        contentWidth: availableWidth

                        ColumnLayout {
                            width: pitchSettingsPage.availableWidth
                            spacing: 14

                            Label {
                                text: "ピッチ加工"
                                font.pixelSize: 18
                                font.bold: true
                            }
                            Switch {
                                id: applyPitchCheck
                                checked: settingsWindow.pendingApplyPitch
                                contentItem: Text {
                                    text: applyPitchCheck.checked ? "ON" : "OFF"
                                    color: applyPitchCheck.checked ? window.accent : window.mutedText
                                    verticalAlignment: Text.AlignVCenter
                                    leftPadding: applyPitchCheck.indicator.width + applyPitchCheck.spacing
                                }
                                text: "予測・手動ピッチを音声へ適用"
                                onToggled: settingsWindow.pendingApplyPitch = checked
                            }
                            Label {
                                text: applyPitchCheck.checked ? "ON: ピッチベントを合成に反映します" : "OFF: ピッチベントを合成に反映しません"
                                color: applyPitchCheck.checked ? window.accent : window.mutedText
                            }
                            Label {
                                Layout.fillWidth: true
                                text: "無効時は原音の声質と発音を優先します。有効時は選択したモデルと下部のピッチ編集をRendererへ渡します。"
                                color: window.mutedText
                                wrapMode: Text.Wrap
                            }
                        }
                    }

                    ScrollView {
                        id: appearanceSettingsPage
                        contentWidth: availableWidth

                        ColumnLayout {
                            width: appearanceSettingsPage.availableWidth
                            spacing: 14

                            Label {
                                text: "外観"
                                font.pixelSize: 18
                                font.bold: true
                            }
                            RowLayout {
                                Layout.fillWidth: true
                                Label {
                                    Layout.fillWidth: true
                                    text: "アプリケーションのテーマ"
                                }
                                Switch {
                                    checked: settingsWindow.pendingDarkMode
                                    text: checked ? "ダークモード" : "ライトモード"
                                    onToggled: settingsWindow.pendingDarkMode = checked
                                }
                            }
                            Label {
                                Layout.fillWidth: true
                                text: "ライトモードが初期設定です。変更は保存時に反映され、次回起動後も維持されます。"
                                color: window.mutedText
                                wrapMode: Text.Wrap
                            }
                        }
                    }

                    ScrollView {
                        id: logSettingsPage
                        contentWidth: availableWidth

                        ColumnLayout {
                            width: logSettingsPage.availableWidth
                            spacing: 14

                            Label {
                                text: "ログ"
                                font.pixelSize: 18
                                font.bold: true
                            }
                            RowLayout {
                                Layout.fillWidth: true
                                Label {
                                    Layout.fillWidth: true
                                    text: "音声合成に成功したときログウィンドウを閉じる"
                                }
                                Switch {
                                    checked: settingsWindow.pendingCloseLogOnSuccess
                                    text: checked ? "ON" : "OFF"
                                    onToggled: settingsWindow.pendingCloseLogOnSuccess = checked
                                }
                            }
                            Label {
                                Layout.fillWidth: true
                                text: settingsWindow.pendingCloseLogOnSuccess ? "成功時は自動的に閉じます。失敗時はログを確認できます。" : "成功してもログウィンドウを開いたままにします。"
                                color: window.mutedText
                                wrapMode: Text.Wrap
                            }
                        }
                    }
                }

                RowLayout {
                    Layout.fillWidth: true
                    Item {
                        Layout.fillWidth: true
                    }
                    Button {
                        text: "保存"
                        onClicked: window.saveSettings()
                    }
                }
            }
        }
    }

    MessageDialog {
        id: aboutDialog
        title: "UtauTTSについて"
        text: "UtauTTS " + Qt.application.version + "by yh"
        informativeText: "UTAUボイスバンクの原音の選択と自然なつなぎ方を学習する日本語TTSソフトウェア"
        buttons: MessageDialog.Ok
    }

    ApplicationWindow {
        id: licenseWindow
        title: "ライセンス"
        visible: false
        width: 860
        height: 620
        minimumWidth: 620
        minimumHeight: 420
        transientParent: window
        modality: Qt.ApplicationModal
        flags: Qt.Dialog
        palette: window.palette
        color: palette.window

        RowLayout {
            anchors.fill: parent
            anchors.margins: 10
            spacing: 8

            ListView {
                id: licenseList
                Layout.preferredWidth: 210
                Layout.fillHeight: true
                clip: true
                model: window.licenseDocuments
                currentIndex: 0

                delegate: ItemDelegate {
                    required property int index
                    required property var modelData
                    width: ListView.view.width
                    text: modelData.name
                    highlighted: ListView.isCurrentItem
                    onClicked: licenseList.currentIndex = index
                }
            }

            ScrollView {
                Layout.fillWidth: true
                Layout.fillHeight: true
                TextArea {
                    width: parent.width
                    text: window.licenseDocuments.length && licenseList.currentIndex >= 0 ? window.licenseDocuments[licenseList.currentIndex].text : ""
                    readOnly: true
                    selectByMouse: true
                    wrapMode: TextEdit.Wrap
                }
            }
        }
    }

    ApplicationWindow {
        id: voicebankDetailsWindow
        title: "ボイスバンクの詳細"
        visible: false
        width: 860
        height: 620
        minimumWidth: 620
        minimumHeight: 420
        transientParent: window
        modality: Qt.ApplicationModal
        flags: Qt.Dialog
        palette: window.palette
        color: palette.window
        property var selectedVoicebank: window.appBackend.voicebanks.length && voicebankDetailsList.currentIndex >= 0 && voicebankDetailsList.currentIndex < window.appBackend.voicebanks.length ? window.appBackend.voicebanks[voicebankDetailsList.currentIndex] : null

        RowLayout {
            anchors.fill: parent
            anchors.margins: 10
            spacing: 8

            ListView {
                id: voicebankDetailsList
                Layout.preferredWidth: 210
                Layout.fillHeight: true
                clip: true
                model: window.appBackend.voicebanks
                currentIndex: 0

                delegate: ItemDelegate {
                    required property int index
                    required property var modelData
                    width: ListView.view.width
                    text: modelData.name
                    highlighted: ListView.isCurrentItem
                    onClicked: voicebankDetailsList.currentIndex = index
                }
            }

            ColumnLayout {
                Layout.fillWidth: true
                Layout.fillHeight: true
                spacing: 10

                Label {
                    Layout.fillWidth: true
                    text: voicebankDetailsWindow.selectedVoicebank ? voicebankDetailsWindow.selectedVoicebank.name : ""
                    font.pixelSize: 18
                    font.bold: true
                }
                Label {
                    Layout.fillWidth: true
                    text: "readme.txt"
                    font.bold: true
                }
                ScrollView {
                    id: voicebankReadmeScroll
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    contentWidth: availableWidth
                    Label {
                        width: voicebankReadmeScroll.availableWidth
                        text: voicebankDetailsWindow.selectedVoicebank ? (voicebankDetailsWindow.selectedVoicebank.readme_text || "READMEがありません") : ""
                        wrapMode: Text.Wrap
                        padding: 4
                    }
                }
            }
        }
    }

    Connections {
        target: window.appBackend

        function onMetadataChanged() {
            window.assignDefaultVoicebank();
            window.assignDefaultSynthesisSettings();
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
            const morae = window.copySequence(analysis.morae);
            const values = [];
            for (let i = 0; i < morae.length; ++i)
                values.push(i < oldPoints.length ? oldPoints[i] : 0);
            utterances.setProperty(index, "reading", analysis.reading);
            utterances.setProperty(index, "moraeJson", JSON.stringify(morae));
            utterances.setProperty(index, "pointsJson", JSON.stringify(values));
            if (index === window.selectedIndex) {
                pitchEditor.points = values.slice();
                pitchEditor.morae = morae.slice();
            }
        }

        function onPreviewReady() {
            const audio = window.appBackend.previewUrl;
            const index = window.utteranceIndex(window.pendingUtteranceId);
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
    }

    Component.onCompleted: addUtterance()

    menuBar: MenuBar {
        Menu {
            title: "ファイル"
            MenuItem {
                text: "WAVを保存..."
                enabled: window.hasCurrentAudio()
                onTriggered: saveDialog.open()
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
            title: "設定"
            MenuItem {
                text: "設定..."
                onTriggered: {
                    window.showAuxiliaryWindow(settingsWindow);
                    settingsWindow.loadCurrent();
                }
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
                    anchors.bottomMargin: 58
                    model: utterances
                    clip: true
                    spacing: 4
                    boundsBehavior: Flickable.StopAtBounds

                    delegate: Item {
                        id: card
                        required property int index
                        required property string content
                        required property string voicebankId
                        required property string imagePath

                        width: ListView.view.width
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
                                placeholderText: "読み上げる文章を入力"
                                selectByMouse: true

                                onActiveFocusChanged: if (activeFocus)
                                    window.selectUtterance(card.index)
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
                                utterances.move(from, to, 1);
                                window.selectedIndex = to;
                                window.draggedUtteranceIndex = to;
                            }
                        }
                    }
                }

                RoundButton {
                    anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    anchors.margins: 8
                    width: 48
                    height: 48
                    text: "+"
                    font.pixelSize: 24
                    highlighted: true
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
                                to: 100
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
                                to: 1
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
                    onPointsEdited: points => window.updatePitchPoints(points)
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
                        Layout.preferredWidth: 42
                        Layout.preferredHeight: 42
                        text: window.appBackend.busy ? "…" : player.playbackState === MediaPlayer.PlayingState ? "Ⅱ" : "▶"
                        highlighted: true
                        enabled: player.playbackState === MediaPlayer.PlayingState || window.hasCurrentAudio() || (!window.appBackend.busy && utterances.count && window.current().content.trim().length > 0)
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

    function showVoicebankDetails() {
        if (!window.appBackend.voicebanks.length)
            return;
        voicebankDetailsList.currentIndex = Math.max(0, Math.min(voicebankDetailsList.currentIndex, window.appBackend.voicebanks.length - 1));
        window.showAuxiliaryWindow(voicebankDetailsWindow);
    }

    function saveSettings() {
        if (utterances.count) {
            window.updateSetting("moraDuration", settingsWindow.pendingMoraDuration);
            window.updateSetting("pauseDuration", settingsWindow.pendingPauseDuration);
            window.updateSetting("applyPitch", settingsWindow.pendingApplyPitch);
            window.selectUtterance(window.selectedIndex);
        }
        window.appBackend.setSynthesisDefaults(settingsWindow.pendingMoraDuration, settingsWindow.pendingPauseDuration, settingsWindow.pendingApplyPitch);
        window.appBackend.setDarkMode(settingsWindow.pendingDarkMode);
        window.appBackend.setCloseLogOnSuccess(settingsWindow.pendingCloseLogOnSuccess);
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
        markUtteranceDirty(selectedIndex);
    }

    function updateUtteranceText(index, text) {
        if (index < 0 || index >= utterances.count)
            return;
        utterances.setProperty(index, "content", text);
        utterances.setProperty(index, "reading", "");
        utterances.setProperty(index, "moraeJson", "[]");
        utterances.setProperty(index, "pointsJson", "[]");
        markUtteranceDirty(index);
        selectUtterance(index);
        analyzeTimer.restart();
    }

    function updatePitchPoints(points) {
        if (!utterances.count)
            return;
        utterances.setProperty(selectedIndex, "pointsJson", JSON.stringify(points));
        if (!current().applyPitch) {
            utterances.setProperty(selectedIndex, "applyPitch", true);
            settingsWindow.pendingApplyPitch = true;
        }
        markUtteranceDirty(selectedIndex);
    }

    function markUtteranceDirty(index) {
        if (index < 0 || index >= utterances.count)
            return;
        const item = utterances.get(index);
        utterances.setProperty(index, "revision", item.revision + 1);
        if (window.audioUtteranceId === item.utteranceId)
            clearPlayback();
    }

    function hasCurrentAudio() {
        if (!utterances.count || !window.audioUtteranceId || !player.source.toString().length)
            return false;
        const item = current();
        return item.utteranceId === window.audioUtteranceId && item.revision === window.audioRevision;
    }

    function clearPlayback() {
        window.playbackRequested = false;
        window.playbackError = "";
        player.stop();
        player.source = "";
        window.audioUtteranceId = "";
        window.audioRevision = -1;
    }

    function assignDefaultVoicebank() {
        if (!utterances.count || !window.appBackend.voicebanks.length)
            return;
        for (let i = 0; i < utterances.count; ++i) {
            const item = utterances.get(i);
            if (!item.voicebankId || !window.voicebankById(item.voicebankId)) {
                utterances.setProperty(i, "voicebankId", window.appBackend.voicebanks[0].id);
                utterances.setProperty(i, "imagePath", window.appBackend.voicebanks[0].image_path || "");
                markUtteranceDirty(i);
            }
        }
        selectUtterance(selectedIndex);
    }

    function assignDefaultSynthesisSettings() {
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
                markUtteranceDirty(index);
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

    function selectUtterance(index) {
        if (index < 0 || index >= utterances.count)
            return;
        const changed = index !== selectedIndex;
        if (changed)
            clearPlayback();
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
        pitchEditor.morae = window.decodeSequence(item.moraeJson);
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

    function addUtterance() {
        const voice = window.appBackend.voicebanks.length ? window.appBackend.voicebanks[0] : null;
        utterances.append({
            utteranceId: "utterance-" + nextUtteranceId++,
            content: "",
            reading: "",
            moraeJson: "[]",
            pointsJson: "[]",
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
        selectUtterance(utterances.count - 1);
        utteranceList.positionViewAtEnd();
    }

    function removeUtterance() {
        clearPlayback();
        utterances.remove(selectedIndex);
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
        utterances.move(selectedIndex, target, 1);
        selectedIndex = target;
        utteranceList.positionViewAtIndex(target, ListView.Contain);
    }

    function synthesizeCurrent() {
        const item = current();
        const points = window.decodeSequence(item.pointsJson);
        const morae = window.decodeSequence(item.moraeJson);
        clearPlayback();
        window.pendingUtteranceId = item.utteranceId;
        window.pendingRevision = item.revision;
        const request = {
            text: item.content,
            voicebank_id: item.voicebankId || voiceCombo.currentValue,
            model_id: item.modelId,
            renderer: item.renderer,
            tone: item.tone,
            mora_duration_ms: item.moraDuration,
            pause_duration_ms: item.pauseDuration,
            intonation_strength: item.intonation,
            apply_pitch: item.applyPitch
        };
        if (item.applyPitch && points.some(value => Math.abs(value) > .1)) {
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
        window.appBackend.clearLogs();
        window.showAuxiliaryWindow(synthesisLogWindow);
        window.appBackend.synthesize(request);
    }
}
