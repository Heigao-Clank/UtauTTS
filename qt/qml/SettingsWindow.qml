pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: root
    required property var hostWindow
    required property var hostPalette
    required property var backend
    signal saveRequested()

    title: "設定"
    visible: false
    width: 720
    height: 540
    minimumWidth: 580
    minimumHeight: 420
    transientParent: hostWindow
    modality: Qt.ApplicationModal
    flags: Qt.Dialog
    palette: hostPalette
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
    property string pendingAddUtteranceShortcut: "Ctrl+D"
    property string pendingRemoveUtteranceShortcut: "Delete"

    function loadCurrent() {
        pendingMoraDuration = root.backend.defaultMoraDuration;
        pendingPauseDuration = root.backend.defaultPauseDuration;
        pendingApplyPitch = root.backend.defaultApplyPitch;
        pendingDarkMode = root.backend.darkMode;
        pendingCloseLogOnSuccess = root.backend.closeLogOnSuccess;
        pendingSynthesizeShortcut = root.backend.synthesizeShortcut;
        pendingSaveProjectShortcut = root.backend.saveProjectShortcut;
        pendingReloadVoicebanksShortcut = root.backend.reloadVoicebanksShortcut;
        pendingAddUtteranceShortcut = root.backend.addUtteranceShortcut;
        pendingRemoveUtteranceShortcut = root.backend.removeUtteranceShortcut;
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
            currentIndex: root.currentPage

            delegate: ItemDelegate {
                required property int index
                required property string modelData
                width: ListView.view.width
                text: modelData
                highlighted: ListView.isCurrentItem
                onClicked: root.currentPage = index
            }
        }

        Rectangle {
            Layout.preferredWidth: 1
            Layout.fillHeight: true
            color: root.hostWindow.borderColor
        }

        ColumnLayout {
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: 10

            StackLayout {
                Layout.fillWidth: true
                Layout.fillHeight: true
                currentIndex: root.currentPage

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
                                    value: root.pendingMoraDuration
                                    editable: true
                                    textFromValue: value => value + " ms"
                                    valueFromText: text => parseInt(text)
                                    onValueModified: root.pendingMoraDuration = value
                                    TapHandler {
                                        acceptedButtons: Qt.LeftButton
                                        grabPermissions: PointerHandler.CanTakeOverFromAnything
                                        onDoubleTapped: root.pendingMoraDuration = 120
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
                                    value: root.pendingPauseDuration
                                    editable: true
                                    textFromValue: value => value + " ms"
                                    valueFromText: text => parseInt(text)
                                    onValueModified: root.pendingPauseDuration = value
                                    TapHandler {
                                        acceptedButtons: Qt.LeftButton
                                        grabPermissions: PointerHandler.CanTakeOverFromAnything
                                        onDoubleTapped: root.pendingPauseDuration = 180
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
                                    checked: root.pendingApplyPitch
                                    onToggled: root.pendingApplyPitch = checked
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
                                currentIndex: root.pendingDarkMode ? 1 : 0
                                onActivated: root.pendingDarkMode = currentIndex === 1
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
                                checked: root.pendingCloseLogOnSuccess
                                onToggled: root.pendingCloseLogOnSuccess = checked
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
                                text: root.pendingSynthesizeShortcut
                                readOnly: true
                                selectByMouse: false
                                onActiveFocusChanged: if (activeFocus) selectAll()
                                Keys.onPressed: event => {
                                    if (event.key === Qt.Key_Backspace || event.key === Qt.Key_Delete) {
                                        root.pendingSynthesizeShortcut = "";
                                        event.accepted = true;
                                        return;
                                    }
                                    const sequence = root.shortcutFromEvent(event);
                                    if (sequence.length) {
                                        root.pendingSynthesizeShortcut = sequence;
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
                                text: root.pendingSaveProjectShortcut
                                readOnly: true
                                selectByMouse: false
                                onActiveFocusChanged: if (activeFocus) selectAll()
                                Keys.onPressed: event => {
                                    if (event.key === Qt.Key_Backspace || event.key === Qt.Key_Delete) {
                                        root.pendingSaveProjectShortcut = "";
                                        event.accepted = true;
                                        return;
                                    }
                                    const sequence = root.shortcutFromEvent(event);
                                    if (sequence.length) {
                                        root.pendingSaveProjectShortcut = sequence;
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
                                text: root.pendingReloadVoicebanksShortcut
                                readOnly: true
                                selectByMouse: false
                                onActiveFocusChanged: if (activeFocus) selectAll()
                                Keys.onPressed: event => {
                                    if (event.key === Qt.Key_Backspace || event.key === Qt.Key_Delete) {
                                        root.pendingReloadVoicebanksShortcut = "";
                                        event.accepted = true;
                                        return;
                                    }
                                    const sequence = root.shortcutFromEvent(event);
                                    if (sequence.length) {
                                        root.pendingReloadVoicebanksShortcut = sequence;
                                        event.accepted = true;
                                    }
                                }
                            }
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Label {
                                Layout.fillWidth: true
                                text: "テキスト追加"
                            }
                            TextField {
                                id: addUtteranceShortcutField
                                Layout.preferredWidth: 180
                                text: root.pendingAddUtteranceShortcut
                                readOnly: true
                                selectByMouse: false
                                onActiveFocusChanged: if (activeFocus) selectAll()
                                Keys.onPressed: event => {
                                    if (event.key === Qt.Key_Backspace || event.key === Qt.Key_Delete) {
                                        root.pendingAddUtteranceShortcut = "";
                                        event.accepted = true;
                                        return;
                                    }
                                    const sequence = root.shortcutFromEvent(event);
                                    if (sequence.length) {
                                        root.pendingAddUtteranceShortcut = sequence;
                                        event.accepted = true;
                                    }
                                }
                            }
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Label {
                                Layout.fillWidth: true
                                text: "テキスト削除"
                            }
                            TextField {
                                id: removeUtteranceShortcutField
                                Layout.preferredWidth: 180
                                text: root.pendingRemoveUtteranceShortcut
                                readOnly: true
                                selectByMouse: false
                                onActiveFocusChanged: if (activeFocus) selectAll()
                                Keys.onPressed: event => {
                                    if (event.key === Qt.Key_Backspace) {
                                        root.pendingRemoveUtteranceShortcut = "";
                                        event.accepted = true;
                                        return;
                                    }
                                    const sequence = root.shortcutFromEvent(event);
                                    if (sequence.length) {
                                        root.pendingRemoveUtteranceShortcut = sequence;
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
                    onClicked: root.saveRequested()
                }
            }
        }
    }
}
