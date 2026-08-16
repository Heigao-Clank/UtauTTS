pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Window {
    id: root
    required property var hostPalette
    required property var backend
    required property var files
    required property url exportDirectory
    required property bool ready
    required property color accent
    required property color mutedText
    signal dragError()

    title: "音声をドラッグ"
    visible: false
    width: 720
    height: 420
    minimumWidth: 580
    minimumHeight: 360
    modality: Qt.NonModal
    flags: Qt.Window
    palette: hostPalette
    color: palette.window

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 16
        spacing: 12

        Label {
            Layout.fillWidth: true
            text: "下の領域をドラッグして、タイムライン上で離してください。"
            wrapMode: Text.WordWrap
        }

        Rectangle {
            id: dragSourceArea
            Layout.fillWidth: true
            Layout.fillHeight: true
            Layout.minimumHeight: 130
            color: palette.alternateBase
            border.width: 2
            border.color: root.accent

            Label {
                anchors.centerIn: parent
                text: "タイムラインへドラッグ"
                horizontalAlignment: Text.AlignHCenter
                color: palette.text
            }

            MouseArea {
                id: dragSourceMouseArea
                anchors.fill: parent
                enabled: root.ready && !root.backend.busy
                property point pressPosition: Qt.point(0, 0)
                property bool pressActive: false

                onPressed: mouse => {
                    pressPosition = Qt.point(mouse.x, mouse.y);
                    pressActive = true;
                }
                onPositionChanged: mouse => {
                    if (!pressActive)
                        return;
                    const dx = mouse.x - pressPosition.x;
                    const dy = mouse.y - pressPosition.y;
                    if (Math.sqrt(dx * dx + dy * dy) < 8)
                        return;
                    pressActive = false;
                    if (!root.backend.startFileDrag(root.files))
                        root.dragError();
                }
                onReleased: pressActive = false
                onCanceled: pressActive = false
            }
        }

        Label {
            Layout.fillWidth: true
            text: "保存先: " + root.exportDirectory.toString()
            elide: Text.ElideMiddle
            color: root.mutedText
        }

        RowLayout {
            Layout.fillWidth: true

            Item { Layout.fillWidth: true }

            Button {
                text: "閉じる"
                onClicked: root.close()
            }
        }
    }
}
