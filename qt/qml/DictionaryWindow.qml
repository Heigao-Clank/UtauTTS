pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ApplicationWindow {
    id: root
    required property var hostWindow
    required property var hostPalette
    required property var backend
    required property var translator

    title: root.translator.tr("dictionary.title")
    visible: false
    width: 760
    height: 560
    minimumWidth: 620
    minimumHeight: 420
    transientParent: hostWindow
    modality: Qt.ApplicationModal
    flags: Qt.Dialog
    palette: hostPalette
    color: palette.window

    ListModel {
        id: dictionaryEntriesModel
    }

    function loadCurrent() {
        dictionaryEntriesModel.clear();
        const entries = root.backend.dictionaryEntries;
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
        root.backend.setDictionaryEntries(entries);
        root.hostWindow.reanalyzeAll();
        root.close();
        root.visible = false;
    }

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 12
        spacing: 10

        Label {
            Layout.fillWidth: true
            text: root.translator.tr("dictionary.description")
            wrapMode: Text.WordWrap
        }

        RowLayout {
            Layout.fillWidth: true
            spacing: 8

            Label {
                Layout.preferredWidth: 280
                text: root.translator.tr("dictionary.surface")
                font.bold: true
            }
            Label {
                Layout.fillWidth: true
                text: root.translator.tr("dictionary.reading")
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
                    placeholderText: root.translator.tr("dictionary.surfaceExample")
                    text: dictionaryEntryRow.surface
                    selectByMouse: true
                    onTextEdited: dictionaryEntriesModel.setProperty(dictionaryEntryRow.index, "surface", text)
                }

                TextField {
                    Layout.fillWidth: true
                    placeholderText: root.translator.tr("dictionary.readingExample")
                    text: dictionaryEntryRow.reading
                    selectByMouse: true
                    onTextEdited: dictionaryEntriesModel.setProperty(dictionaryEntryRow.index, "reading", text)
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
                    onClicked: dictionaryEntriesModel.remove(dictionaryEntryRow.index)
                    ToolTip.visible: hovered
                    ToolTip.text: root.translator.tr("dictionary.delete")
                }
            }
        }

        RowLayout {
            Layout.fillWidth: true

            Button {
                text: root.translator.tr("dictionary.addEntry")
                onClicked: root.addEntry()
            }

            Item {
                Layout.fillWidth: true
            }

            Button {
                text: root.translator.tr("common.cancel")
                onClicked: {
                    root.close();
                    root.visible = false;
                }
            }

            Button {
                text: root.translator.tr("common.save")
                highlighted: true
                onClicked: root.saveCurrent()
            }
        }
    }
}
