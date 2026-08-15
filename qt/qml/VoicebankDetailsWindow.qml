pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: root
    required property var hostWindow
    required property var hostPalette
    required property var backend
    property alias currentIndex: voicebankDetailsList.currentIndex
    property var selectedVoicebank: root.backend.voicebanks.length && voicebankDetailsList.currentIndex >= 0 && voicebankDetailsList.currentIndex < root.backend.voicebanks.length ? root.backend.voicebanks[voicebankDetailsList.currentIndex] : null

    title: "ボイスバンクの詳細"
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
            id: voicebankDetailsList
            Layout.preferredWidth: 210
            Layout.fillHeight: true
            clip: true
            model: root.backend.voicebanks
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
                text: root.selectedVoicebank ? root.selectedVoicebank.name : ""
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
                    text: root.selectedVoicebank ? (root.selectedVoicebank.readme_text || "READMEがありません") : ""
                    wrapMode: Text.Wrap
                    padding: 4
                }
            }
        }
    }
}
