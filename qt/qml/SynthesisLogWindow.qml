pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Window {
    id: root
    required property var hostPalette
    required property var backend

    title: "音声合成ログ"
    visible: false
    width: 720
    height: 420
    minimumWidth: 520
    minimumHeight: 280
    palette: hostPalette
    color: palette.window

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 12
        spacing: 8

        Label {
            Layout.fillWidth: true
            text: root.backend.busy ? "音声合成を実行中です…" : "音声合成ログ"
            font.bold: true
        }

        ScrollView {
            Layout.fillWidth: true
            Layout.fillHeight: true
            TextArea {
                id: synthesisLogText
                width: root.width - 36
                text: root.backend.logLines.join("\n")
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
                onClicked: root.close()
            }
        }
    }
}
