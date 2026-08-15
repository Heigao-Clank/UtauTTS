pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Window {
    id: root
    required property var hostPalette
    required property color mutedText
    required property url exportDirectory
    signal exportRequested(string target)

    title: "ドロップ先の編集ソフト"
    visible: false
    width: 720
    height: 320
    minimumWidth: 580
    minimumHeight: 280
    modality: Qt.NonModal
    flags: Qt.Window
    palette: hostPalette
    color: palette.window

    function reset() {
        dragTargetCombo.currentIndex = 0;
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 16
        spacing: 12

        Label {
            Layout.fillWidth: true
            text: "編集ソフトを選択してください。"
            font.bold: true
            wrapMode: Text.WordWrap
        }

        RowLayout {
            Layout.fillWidth: true

            Label {
                text: "ドロップ先"
            }

            ComboBox {
                id: dragTargetCombo
                Layout.fillWidth: true
                model: ["AviUtl", "AviUtl2", "YMM4"]
            }
        }

        Label {
            Layout.fillWidth: true
            text: "保存先: " + root.exportDirectory.toString()
            elide: Text.ElideMiddle
            color: root.mutedText
        }

        Item {
            Layout.fillHeight: true
        }

        RowLayout {
            Layout.fillWidth: true

            Item { Layout.fillWidth: true }

            Button {
                text: "キャンセル"
                onClicked: root.close()
            }

            Button {
                text: "次へ"
                highlighted: true
                onClicked: root.exportRequested(dragTargetCombo.currentText)
            }
        }
    }
}
