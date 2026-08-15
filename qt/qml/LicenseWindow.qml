pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: root
    required property var hostWindow
    required property var hostPalette
    property var documents: []

    title: "ライセンス"
    visible: false
    width: 860
    height: 620
    minimumWidth: 620
    minimumHeight: 420
    transientParent: hostWindow
    modality: Qt.ApplicationModal
    flags: Qt.Dialog
    palette: hostPalette
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
            model: root.documents
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
                text: root.documents.length && licenseList.currentIndex >= 0 ? root.documents[licenseList.currentIndex].text : ""
                readOnly: true
                selectByMouse: true
                wrapMode: TextEdit.Wrap
            }
        }
    }
}
