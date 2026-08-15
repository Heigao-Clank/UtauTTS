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
    property bool saveRequestPending: false
    property bool playbackRequested: false
    property string playbackError: ""
    property bool batchExportActive: false
    property int batchExportIndex: -1
    property int batchExportOriginalIndex: 0
    property int batchExportCompleted: 0
    property url batchExportDirectory
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

    FolderDialog {
        id: saveAllDialog
        onAccepted: window.startBatchExport(selectedFolder)
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
        property string pendingSynthesizeShortcut: "Ctrl+Enter"
        property string pendingSaveProjectShortcut: "Ctrl+S"
        property string pendingReloadVoicebanksShortcut: "Ctrl+O"

        function loadCurrent() {
            pendingMoraDuration = window.appBackend.defaultMoraDuration;
            pendingPauseDuration = window.appBackend.defaultPauseDuration;
            pendingApplyPitch = window.appBackend.defaultApplyPitch;
            pendingDarkMode = window.appBackend.darkMode;
            pendingCloseLogOnSuccess = window.appBackend.closeLogOnSuccess;
            pendingSynthesizeShortcut = window.appBackend.synthesizeShortcut;
            pendingSaveProjectShortcut = window.appBackend.saveProjectShortcut;
            pendingReloadVoicebanksShortcut = window.appBackend.reloadVoicebanksShortcut;
            themeCombo.currentIndex = pendingDarkMode ? 1 : 0;
        }

        function shortcutFromEvent(event) {
            const key = event.key;
            if (key === Qt.Key_Control || key === Qt.Key_Shift || key === Qt.Key_Alt || key === Qt.Key_Meta)
                return "";

            const parts = [];
            if (event.modifiers & Qt.ControlModifier)
                parts.push("Ctrl");
            if (event.modifiers & Qt.AltModifier)
                parts.push("Alt");
            if (event.modifiers & Qt.ShiftModifier)
                parts.push("Shift");
            if (event.modifiers & Qt.MetaModifier)
                parts.push("Meta");

            let keyName = "";
            if (key >= Qt.Key_A && key <= Qt.Key_Z)
                keyName = String.fromCharCode(key);
            else if (key >= Qt.Key_0 && key <= Qt.Key_9)
                keyName = String.fromCharCode(key);
            else if (key >= Qt.Key_F1 && key <= Qt.Key_F35)
                keyName = "F" + (key - Qt.Key_F1 + 1);
            else {
                switch (key) {
                case Qt.Key_Return:
                case Qt.Key_Enter: keyName = "Enter"; break;
                case Qt.Key_Space: keyName = "Space"; break;
                case Qt.Key_Tab:
                case Qt.Key_Backtab: keyName = "Tab"; break;
                case Qt.Key_Escape: keyName = "Esc"; break;
                case Qt.Key_Left: keyName = "Left"; break;
                case Qt.Key_Right: keyName = "Right"; break;
                case Qt.Key_Up: keyName = "Up"; break;
                case Qt.Key_Down: keyName = "Down"; break;
                case Qt.Key_Home: keyName = "Home"; break;
                case Qt.Key_End: keyName = "End"; break;
                case Qt.Key_PageUp: keyName = "PageUp"; break;
                case Qt.Key_PageDown: keyName = "PageDown"; break;
                case Qt.Key_Insert: keyName = "Insert"; break;
                case Qt.Key_Delete: keyName = "Delete"; break;
                case Qt.Key_Plus: keyName = "Plus"; break;
                case Qt.Key_Minus: keyName = "Minus"; break;
                case Qt.Key_Comma: keyName = "Comma"; break;
                case Qt.Key_Period: keyName = "Period"; break;
                }
            }
            return keyName ? parts.concat([keyName]).join("+") : "";
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
                model: ["音声合成", "表示", "ログ", "ショートカット"]
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

                            ColumnLayout {
                                Layout.fillWidth: true
                                spacing: 8

                                RowLayout {
                                    Layout.fillWidth: true
                                    Label {
                                        text: "デフォルトモーラ長"
                                        Layout.fillWidth: true
                                    }
                                    SpinBox {
                                        id: moraSpin
                                        Layout.preferredWidth: 180
                                        Layout.alignment: Qt.AlignVCenter
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
                                }
                                RowLayout {
                                    Layout.fillWidth: true
                                    Label {
                                        text: "デフォルト休止長"
                                        Layout.fillWidth: true
                                    }
                                    SpinBox {
                                        id: pauseSpin
                                        Layout.preferredWidth: 180
                                        Layout.alignment: Qt.AlignVCenter
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
                                RowLayout {
                                    Layout.fillWidth: true
                                    Label {
                                        text: "手動ピッチ変更の許可"
                                        Layout.fillWidth: true
                                    }
                                    Switch {
                                        id: applyPitchCheck
                                        Layout.alignment: Qt.AlignVCenter | Qt.AlignRight
                                        checked: settingsWindow.pendingApplyPitch
                                        onToggled: settingsWindow.pendingApplyPitch = checked
                                    }
                                }
                            }
                        }
                    }

                    ScrollView {
                        id: appearanceSettingsPage
                        contentWidth: availableWidth

                        ColumnLayout {
                            width: appearanceSettingsPage.availableWidth
                            spacing: 8

                            RowLayout {
                                Layout.fillWidth: true
                                Label {
                                    Layout.fillWidth: true
                                    text: "テーマ"
                                }
                                ComboBox {
                                    id: themeCombo
                                    Layout.preferredWidth: 180
                                    model: ["ライト", "ダーク"]
                                    currentIndex: settingsWindow.pendingDarkMode ? 1 : 0
                                    onActivated: settingsWindow.pendingDarkMode = currentIndex === 1
                                }
                            }
                        }
                    }

                    ScrollView {
                        id: logSettingsPage
                        contentWidth: availableWidth

                        ColumnLayout {
                            width: logSettingsPage.availableWidth
                            spacing: 8

                            RowLayout {
                                Layout.fillWidth: true
                                Label {
                                    Layout.fillWidth: true
                                    text: "成功時にログを閉じる"
                                }
                                Switch {
                                    Layout.alignment: Qt.AlignVCenter | Qt.AlignRight
                                    checked: settingsWindow.pendingCloseLogOnSuccess
                                    onToggled: settingsWindow.pendingCloseLogOnSuccess = checked
                                }
                            }
                        }
                    }

                    ScrollView {
                        id: shortcutSettingsPage
                        contentWidth: availableWidth

                        ColumnLayout {
                            width: shortcutSettingsPage.availableWidth
                            spacing: 12

                            Label {
                                Layout.fillWidth: true
                                text: "変更したい欄を選択して、割り当てるキーを押してください。Backspaceで無効にできます。"
                                wrapMode: Text.WordWrap
                            }

                            RowLayout {
                                Layout.fillWidth: true
                                Label {
                                    Layout.fillWidth: true
                                    text: "音声合成"
                                }
                                TextField {
                                    id: synthesizeShortcutField
                                    Layout.preferredWidth: 180
                                    text: settingsWindow.pendingSynthesizeShortcut
                                    readOnly: true
                                    selectByMouse: false
                                    onActiveFocusChanged: if (activeFocus) selectAll()
                                    Keys.onPressed: event => {
                                        if (event.key === Qt.Key_Backspace || event.key === Qt.Key_Delete) {
                                            settingsWindow.pendingSynthesizeShortcut = "";
                                            event.accepted = true;
                                            return;
                                        }
                                        const sequence = settingsWindow.shortcutFromEvent(event);
                                        if (sequence.length) {
                                            settingsWindow.pendingSynthesizeShortcut = sequence;
                                            event.accepted = true;
                                        }
                                    }
                                }
                            }

                            RowLayout {
                                Layout.fillWidth: true
                                Label {
                                    Layout.fillWidth: true
                                    text: "プロジェクト保存"
                                }
                                TextField {
                                    id: saveProjectShortcutField
                                    Layout.preferredWidth: 180
                                    text: settingsWindow.pendingSaveProjectShortcut
                                    readOnly: true
                                    selectByMouse: false
                                    onActiveFocusChanged: if (activeFocus) selectAll()
                                    Keys.onPressed: event => {
                                        if (event.key === Qt.Key_Backspace || event.key === Qt.Key_Delete) {
                                            settingsWindow.pendingSaveProjectShortcut = "";
                                            event.accepted = true;
                                            return;
                                        }
                                        const sequence = settingsWindow.shortcutFromEvent(event);
                                        if (sequence.length) {
                                            settingsWindow.pendingSaveProjectShortcut = sequence;
                                            event.accepted = true;
                                        }
                                    }
                                }
                            }

                            RowLayout {
                                Layout.fillWidth: true
                                Label {
                                    Layout.fillWidth: true
                                    text: "ボイスバンク再読み込み"
                                }
                                TextField {
                                    id: reloadVoicebanksShortcutField
                                    Layout.preferredWidth: 180
                                    text: settingsWindow.pendingReloadVoicebanksShortcut
                                    readOnly: true
                                    selectByMouse: false
                                    onActiveFocusChanged: if (activeFocus) selectAll()
                                    Keys.onPressed: event => {
                                        if (event.key === Qt.Key_Backspace || event.key === Qt.Key_Delete) {
                                            settingsWindow.pendingReloadVoicebanksShortcut = "";
                                            event.accepted = true;
                                            return;
                                        }
                                        const sequence = settingsWindow.shortcutFromEvent(event);
                                        if (sequence.length) {
                                            settingsWindow.pendingReloadVoicebanksShortcut = sequence;
                                            event.accepted = true;
                                        }
                                    }
                                }
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

    ApplicationWindow {
        id: dictionaryWindow
        title: "辞書設定"
        visible: false
        width: 760
        height: 560
        minimumWidth: 620
        minimumHeight: 420
        transientParent: window
        modality: Qt.ApplicationModal
        flags: Qt.Dialog
        palette: window.palette
        color: palette.window

        ListModel {
            id: dictionaryEntriesModel
        }

        function loadCurrent() {
            dictionaryEntriesModel.clear();
            const entries = window.appBackend.dictionaryEntries;
            for (let index = 0; index < entries.length; ++index) {
                const entry = entries[index] || {};
                dictionaryEntriesModel.append({
                    surface: String(entry.surface || ""),
                    reading: String(entry.reading || "")
                });
            }
        }

        function addEntry() {
            dictionaryEntriesModel.append({surface: "", reading: ""});
            dictionaryList.positionViewAtEnd();
        }

        function saveCurrent() {
            const entries = [];
            for (let index = 0; index < dictionaryEntriesModel.count; ++index) {
                const entry = dictionaryEntriesModel.get(index);
                entries.push({
                    surface: String(entry.surface || "").trim(),
                    reading: String(entry.reading || "").trim()
                });
            }
            window.appBackend.setDictionaryEntries(entries);
            window.reanalyzeAll();
            dictionaryWindow.close();
            dictionaryWindow.visible = false;
        }

        ColumnLayout {
            anchors.fill: parent
            anchors.margins: 12
            spacing: 10

            Label {
                Layout.fillWidth: true
                text: "文章中の表記を、指定した読みへ置き換えます。読みはひらがなまたはカタカナで入力してください。"
                wrapMode: Text.WordWrap
            }

            RowLayout {
                Layout.fillWidth: true
                spacing: 8

                Label {
                    Layout.preferredWidth: 280
                    text: "表記"
                    font.bold: true
                }
                Label {
                    Layout.fillWidth: true
                    text: "読み"
                    font.bold: true
                }
                Item {
                    Layout.preferredWidth: 32
                }
            }

            ListView {
                id: dictionaryList
                Layout.fillWidth: true
                Layout.fillHeight: true
                clip: true
                spacing: 2
                model: dictionaryEntriesModel
                ScrollBar.vertical: ScrollBar {
                    id: dictionaryScrollBar
                    policy: ScrollBar.AlwaysOn
                }

                delegate: RowLayout {
                    id: dictionaryEntryRow
                    width: Math.max(0, dictionaryList.width - 14 - 2)
                    height: 36
                    spacing: 4

                    required property int index
                    required property string surface
                    required property string reading

                    TextField {
                        Layout.preferredWidth: 280
                        placeholderText: "例: UtauTTS"
                        text: dictionaryEntryRow.surface
                        selectByMouse: true
                        onTextEdited: dictionaryEntriesModel.setProperty(index, "surface", text)
                    }

                    TextField {
                        Layout.fillWidth: true
                        placeholderText: "例: うたうてぃーてぃーえす"
                        text: dictionaryEntryRow.reading
                        selectByMouse: true
                        onTextEdited: dictionaryEntriesModel.setProperty(index, "reading", text)
                    }

                    ToolButton {
                        id: dictionaryDeleteButton
                        Layout.preferredWidth: 32
                        Layout.minimumWidth: 32
                        Layout.maximumWidth: 32
                        Layout.preferredHeight: 32
                        Layout.alignment: Qt.AlignVCenter
                        contentItem: Canvas {
                            id: dictionaryDeleteIcon
                            anchors.centerIn: parent
                            width: 22
                            height: 22
                            property color iconColor: dictionaryDeleteButton.palette.buttonText
                            onIconColorChanged: requestPaint()
                            onPaint: {
                                const context = getContext("2d");
                                context.clearRect(0, 0, width, height);
                                context.strokeStyle = iconColor;
                                context.lineWidth = 1.8;
                                context.lineCap = "round";
                                context.lineJoin = "round";
                                context.beginPath();
                                context.moveTo(width * 0.29, height * 0.31);
                                context.lineTo(width * 0.71, height * 0.31);
                                context.moveTo(width * 0.41, height * 0.24);
                                context.lineTo(width * 0.59, height * 0.24);
                                context.moveTo(width * 0.37, height * 0.31);
                                context.lineTo(width * 0.41, height * 0.76);
                                context.lineTo(width * 0.59, height * 0.76);
                                context.lineTo(width * 0.63, height * 0.31);
                                context.stroke();
                            }
                        }
                        onClicked: dictionaryEntriesModel.remove(index)
                        ToolTip.visible: hovered
                        ToolTip.text: "削除"
                    }
                }
            }

            RowLayout {
                Layout.fillWidth: true

                Button {
                    text: "項目を追加"
                    onClicked: dictionaryWindow.addEntry()
                }

                Item {
                    Layout.fillWidth: true
                }

                Button {
                    text: "キャンセル"
                    onClicked: {
                        dictionaryWindow.close();
                        dictionaryWindow.visible = false;
                    }
                }

                Button {
                    text: "保存"
                    highlighted: true
                    onClicked: dictionaryWindow.saveCurrent()
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
            if (index === window.selectedIndex) {
                pitchEditor.points = values.slice();
                pitchEditor.morae = morae.slice();
                pitchEditor.moraDurations = durations.slice();
                pitchEditor.moraPositions = positions.slice();
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
                const destination = window.appBackend.fileInDirectory(
                            window.batchExportDirectory, window.audioFileName(utterances.get(index)));
                window.pendingUtteranceId = "";
                window.pendingRevision = -1;
                if (!destination.toString().length || !window.appBackend.savePreview(destination)) {
                    window.finishBatchExport(false);
                    return;
                }
                ++window.batchExportCompleted;
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
        voicebankDetailsList.currentIndex = Math.max(0, Math.min(voicebankDetailsList.currentIndex, window.appBackend.voicebanks.length - 1));
        window.showAuxiliaryWindow(voicebankDetailsWindow);
    }

    function saveSettings() {
        const shortcuts = [settingsWindow.pendingSynthesizeShortcut,
                           settingsWindow.pendingSaveProjectShortcut,
                           settingsWindow.pendingReloadVoicebanksShortcut];
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
                                               settingsWindow.pendingReloadVoicebanksShortcut);
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
        for (let index = 0; index < loadedUtterances.length; ++index) {
            const saved = loadedUtterances[index] || {};
            const voicebankId = String(saved.voicebank_id || "");
            const voice = window.voicebankById(voicebankId);
            const points = window.copySequence(saved.pitch_points);
            const content = String(saved.text || "");
            utterances.append({
                utteranceId: "utterance-" + window.nextUtteranceId++,
                content: content,
                reading: "",
                moraeJson: "[]",
                pointsJson: JSON.stringify(points),
                moraDurationsJson: JSON.stringify(window.copySequence(saved.mora_durations_ms)),
                moraPositionsJson: JSON.stringify(window.copySequence(saved.mora_positions_ms)),
                voicebankId: voicebankId,
                imagePath: voice ? voice.image_path || "" : "",
                modelId: String(saved.model_id || ""),
                renderer: String(saved.renderer_id || ""),
                tone: String(saved.tone || "C4"),
                moraDuration: window.projectNumber(saved.mora_duration_ms, window.appBackend.defaultMoraDuration, 20, 1000, true),
                pauseDuration: window.projectNumber(saved.pause_duration_ms, window.appBackend.defaultPauseDuration, 0, 3000, true),
                intonation: window.projectNumber(saved.intonation, 0, 0, 1, false),
                applyPitch: saved.apply_pitch === undefined ? window.appBackend.defaultApplyPitch : !!saved.apply_pitch,
                revision: 0
            });
        }

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
        if (name === "moraDuration")
            pitchEditor.defaultMoraDuration = value;
        else if (name === "pauseDuration")
            pitchEditor.defaultPauseDuration = value;
        markUtteranceDirty(selectedIndex);
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

    function updateMoraDurations(durations) {
        if (!utterances.count)
            return;
        utterances.setProperty(selectedIndex, "moraDurationsJson", JSON.stringify(durations));
        markUtteranceDirty(selectedIndex);
    }

    function updateMoraPositions(positions) {
        if (!utterances.count)
            return;
        utterances.setProperty(selectedIndex, "moraPositionsJson", JSON.stringify(positions));
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

    function clearPlayback() {
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
        pitchEditor.defaultMoraDuration = item.moraDuration;
        pitchEditor.defaultPauseDuration = item.pauseDuration;
        pitchEditor.moraDurations = window.decodeSequence(item.moraDurationsJson);
        pitchEditor.moraPositions = window.decodeSequence(item.moraPositionsJson);
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
        selectUtterance(utterances.count - 1);
        utteranceList.positionViewAtEnd();
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
        utterances.move(selectedIndex, target, 1);
        window.projectDirty = true;
        selectedIndex = target;
        utteranceList.positionViewAtIndex(target, ListView.Contain);
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
            mora_durations_ms: window.decodeSequence(item.moraDurationsJson),
            intonation_strength: item.intonation,
            apply_pitch: item.applyPitch
        };
        if (item.applyPitch && item.reading && points.some(value => Math.abs(value) > .1)) {
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

    function startBatchExport(directory) {
        if (!directory || !directory.toString().length || !utterances.count || window.appBackend.busy)
            return;
        window.batchExportDirectory = directory;
        window.batchExportOriginalIndex = window.selectedIndex;
        window.batchExportIndex = 0;
        window.batchExportCompleted = 0;
        window.batchExportActive = true;
        window.clearPlayback();
        window.pendingUtteranceId = "";
        window.pendingRevision = -1;
        window.appBackend.clearLogs();
        window.showAuxiliaryWindow(synthesisLogWindow);
        Qt.callLater(window.synthesizeBatchItem);
    }

    function synthesizeBatchItem() {
        if (!window.batchExportActive)
            return;
        while (window.batchExportIndex < utterances.count) {
            const index = window.batchExportIndex++;
            const item = utterances.get(index);
            if (!item.content || !item.content.trim())
                continue;
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
        window.batchExportActive = false;
        window.pendingUtteranceId = "";
        window.pendingRevision = -1;
        if (utterances.count)
            window.selectUtterance(Math.min(window.batchExportOriginalIndex, utterances.count - 1));
        if (success && window.appBackend.closeLogOnSuccess)
            synthesisLogWindow.close();
    }
}
