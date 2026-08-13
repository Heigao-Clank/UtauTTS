pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQuick.Dialogs
import QtMultimedia

ApplicationWindow {
    id: window
    width: 1180
    height: 820
    visible: true
    title: "UtauTTS"
    color: "#f4f7f5"

    property color accent: "#58a875"
    property int selectedIndex: 0
    property int nextUtteranceId: 1

    ListModel { id: utterances }

    Timer {
        id: analyzeTimer
        interval: 250
        onTriggered: {
            if (utterances.count && window.current().content.trim()) {
                const item = window.current()
                backend.analyze(item.content, item.utteranceId)
            }
        }
    }

    MediaPlayer { id: player; audioOutput: AudioOutput {} }
    FileDialog {
        id: saveDialog
        fileMode: FileDialog.SaveFile
        nameFilters: ["WAV音声 (*.wav)"]
        defaultSuffix: "wav"
        onAccepted: backend.savePreview(selectedFile)
    }

    Connections {
        target: backend

        function onMetadataChanged() {
            window.assignDefaultVoicebank()
        }

        function onAnalysisReady(requestId, sourceText, analysis) {
            const index = window.utteranceIndex(requestId)
            if (index < 0 || utterances.get(index).content !== sourceText)
                return
            const old = utterances.get(index)
            const values = []
            for (let i = 0; i < analysis.morae.length; ++i)
                values.push(old.points && i < old.points.length ? old.points[i] : 0)
            utterances.setProperty(index, "reading", analysis.reading)
            utterances.setProperty(index, "morae", analysis.morae)
            utterances.setProperty(index, "points", values)
            if (index === window.selectedIndex) {
                pitchEditor.points = values
                pitchEditor.morae = analysis.morae
            }
        }

        function onSynthesisReady(audio, result) {
            player.source = audio
            player.play()
        }
    }

    Component.onCompleted: addUtterance()

    header: ToolBar {
        height: 54
        background: Rectangle { color: "white"; border.color: "#cad8ce" }

        RowLayout {
            anchors.fill: parent
            anchors.margins: 8
            spacing: 9

            Label { text: "UtauTTS"; font.pixelSize: 19; font.bold: true; color: "#28633e" }
            Label {
                text: !backend.connected ? "● バックエンド未接続" : backend.error.length ? "● 処理エラー" : "● ネイティブ接続済み"
                color: !backend.connected || backend.error.length ? "#a33b3b" : "#397a4d"
                font.pixelSize: 12
            }
            Label { text: backend.error; color: "#a33b3b"; elide: Text.ElideRight; Layout.fillWidth: true }
            Button { text: "音源再読込"; enabled: !backend.busy; onClicked: backend.reloadVoicebanks() }
            Button { text: "WAV保存"; enabled: player.source.toString().length > 0; onClicked: saveDialog.open() }
            Button {
                text: backend.busy ? "合成中…" : "▶ 生成・再生"
                highlighted: true
                enabled: !backend.busy && utterances.count && window.current().content.trim().length > 0
                onClicked: window.synthesizeCurrent()
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
                SplitView.minimumWidth: 480
                padding: 12
                background: Rectangle { color: "#f4f7f5" }

                ColumnLayout {
                    anchors.fill: parent
                    spacing: 8

                    RowLayout {
                        Layout.fillWidth: true
                        Label { text: "テキスト"; font.bold: true; font.pixelSize: 16; color: "#344139" }
                        Label { text: "テキストボックスごとに音源と設定を保持します"; color: "#718078"; Layout.fillWidth: true }
                        Button { text: "＋ テキストボックスを追加"; onClicked: window.addUtterance() }
                        Button { text: "削除"; enabled: utterances.count > 1; onClicked: window.removeUtterance() }
                        Button { text: "↑"; onClicked: window.moveUtterance(-1) }
                        Button { text: "↓"; onClicked: window.moveUtterance(1) }
                    }

                    ListView {
                        id: utteranceList
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        model: utterances
                        clip: true
                        spacing: 8
                        boundsBehavior: Flickable.StopAtBounds

                        delegate: Frame {
                            id: card
                            required property int index
                            required property string content
                            required property string reading
                            required property string voicebankId
                            required property string imagePath

                            width: ListView.view.width
                            height: 146
                            padding: 10
                            background: Rectangle {
                                radius: 7
                                color: card.index === window.selectedIndex ? "#f5fff8" : "white"
                                border.width: card.index === window.selectedIndex ? 2 : 1
                                border.color: card.index === window.selectedIndex ? window.accent : "#cad8ce"
                            }

                            RowLayout {
                                anchors.fill: parent
                                spacing: 12

                                Rectangle {
                                    Layout.preferredWidth: 88
                                    Layout.fillHeight: true
                                    radius: 5
                                    color: "#edf3ef"
                                    border.color: "#d5e1d8"

                                    Image {
                                        anchors.fill: parent
                                        anchors.margins: 5
                                        source: window.localImageUrl(card.imagePath)
                                        fillMode: Image.PreserveAspectFit
                                        asynchronous: true
                                    }
                                    Label {
                                        anchors.centerIn: parent
                                        visible: !card.imagePath
                                        text: "No Image"
                                        color: "#8a968e"
                                        font.pixelSize: 11
                                    }
                                }

                                ColumnLayout {
                                    Layout.fillWidth: true
                                    Layout.fillHeight: true
                                    spacing: 3

                                    RowLayout {
                                        Layout.fillWidth: true
                                        Label { text: window.voicebankName(card.voicebankId); font.bold: true; color: "#356447" }
                                        Label { text: card.reading || "読みを解析中"; color: "#718078"; elide: Text.ElideRight; Layout.fillWidth: true }
                                        Label { text: "#" + (card.index + 1); color: "#829087" }
                                    }

                                    TextArea {
                                        id: utteranceEditor
                                        Layout.fillWidth: true
                                        Layout.fillHeight: true
                                        text: card.content
                                        font.pixelSize: 18
                                        wrapMode: TextEdit.Wrap
                                        placeholderText: "読み上げる文章を入力してください"
                                        selectByMouse: true
                                        background: Rectangle { color: "transparent" }

                                        onActiveFocusChanged: {
                                            if (activeFocus)
                                                window.selectUtterance(card.index)
                                        }
                                        onTextChanged: {
                                            if (card.index >= utterances.count || utterances.get(card.index).content === text)
                                                return
                                            utterances.setProperty(card.index, "content", text)
                                            window.selectUtterance(card.index)
                                            analyzeTimer.restart()
                                        }
                                    }
                                }
                            }
                        }
                    }

                    RowLayout {
                        visible: player.source.toString().length > 0
                        Layout.fillWidth: true
                        Button {
                            text: player.playbackState === MediaPlayer.PlayingState ? "Ⅱ" : "▶"
                            onClicked: player.playbackState === MediaPlayer.PlayingState ? player.pause() : player.play()
                        }
                        Slider {
                            Layout.fillWidth: true
                            from: 0
                            to: Math.max(1, player.duration)
                            value: player.position
                            onMoved: player.position = value
                        }
                        Label { text: Math.round(player.position / 1000) + " / " + Math.round(player.duration / 1000) + "秒" }
                    }
                }
            }

            Pane {
                SplitView.preferredWidth: 310
                SplitView.minimumWidth: 230
                SplitView.maximumWidth: 520
                padding: 14
                background: Rectangle { color: "#fbfdfb"; border.color: "#cad8ce" }

                ScrollView {
                    anchors.fill: parent
                    contentWidth: availableWidth

                    ColumnLayout {
                        width: parent.width
                        spacing: 12

                        Label { text: "選択中の設定"; font.bold: true; font.pixelSize: 16 }
                        Label { text: "音源" }
                        ComboBox {
                            id: voiceCombo
                            Layout.fillWidth: true
                            model: backend.voicebanks
                            textRole: "name"
                            valueRole: "id"
                            onActivated: {
                                window.updateSetting("voicebankId", currentValue)
                                const voice = window.voicebankById(currentValue)
                                window.updateSetting("imagePath", voice ? voice.image_path : "")
                            }
                        }
                        Label { text: "抑揚モデル" }
                        ComboBox {
                            id: modelCombo
                            Layout.fillWidth: true
                            model: [{id: "none", display_name: "なし"}].concat(backend.models)
                            textRole: "display_name"
                            valueRole: "id"
                            onActivated: window.updateSetting("modelId", currentValue)
                        }
                        Label { text: "Renderer" }
                        ComboBox {
                            id: rendererCombo
                            Layout.fillWidth: true
                            model: [{id: "", display_name: "既定: " + backend.defaultRenderer}].concat(backend.renderers)
                            textRole: "display_name"
                            valueRole: "id"
                            onActivated: window.updateSetting("renderer", currentValue)
                        }
                        Label { text: "音高" }
                        TextField { id: toneField; Layout.fillWidth: true; text: "C4"; onEditingFinished: window.updateSetting("tone", text) }
                        Label { text: "モーラ長 (ms)" }
                        SpinBox { id: moraSpin; Layout.fillWidth: true; from: 20; to: 1000; value: 120; onValueModified: window.updateSetting("moraDuration", value) }
                        Label { text: "休止長 (ms)" }
                        SpinBox { id: pauseSpin; Layout.fillWidth: true; from: 0; to: 3000; value: 180; onValueModified: window.updateSetting("pauseDuration", value) }
                        RowLayout {
                            Layout.fillWidth: true
                            Label { text: "抑揚"; Layout.fillWidth: true }
                            Label { text: intonationSlider.value.toFixed(2) }
                        }
                        Slider { id: intonationSlider; Layout.fillWidth: true; from: 0; to: 1; stepSize: .05; onMoved: window.updateSetting("intonation", value) }
                        CheckBox { id: applyPitchCheck; text: "ピッチ加工を有効にする"; onToggled: window.updateSetting("applyPitch", checked) }
                    }
                }
            }
        }

        Pane {
            SplitView.preferredHeight: 290
            SplitView.minimumHeight: 170
            padding: 12
            background: Rectangle { color: "white"; border.color: "#cad8ce" }

            ColumnLayout {
                anchors.fill: parent
                RowLayout {
                    Layout.fillWidth: true
                    Label { text: "ピッチベント"; font.bold: true }
                    Label { text: "点を上下へドラッグしてcentを調整"; color: "#718078"; font.pixelSize: 11 }
                }
                PitchEditor {
                    id: pitchEditor
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    onPointsEdited: points => utterances.setProperty(window.selectedIndex, "points", points)
                }
            }
        }
    }

    function current() {
        return utterances.get(selectedIndex)
    }

    function voicebankById(id) {
        for (let i = 0; i < backend.voicebanks.length; ++i) {
            if (backend.voicebanks[i].id === id)
                return backend.voicebanks[i]
        }
        return null
    }

    function utteranceIndex(id) {
        for (let i = 0; i < utterances.count; ++i) {
            if (utterances.get(i).utteranceId === id)
                return i
        }
        return -1
    }

    function voicebankName(id) {
        const voice = voicebankById(id)
        return voice ? voice.name : "音源未選択"
    }

    function localImageUrl(path) {
        if (!path)
            return ""
        return encodeURI("file:///" + path.replace(/\\/g, "/"))
    }

    function updateSetting(name, value) {
        if (utterances.count)
            utterances.setProperty(selectedIndex, name, value)
    }

    function assignDefaultVoicebank() {
        if (!utterances.count || !backend.voicebanks.length)
            return
        for (let i = 0; i < utterances.count; ++i) {
            const item = utterances.get(i)
            if (!item.voicebankId) {
                utterances.setProperty(i, "voicebankId", backend.voicebanks[0].id)
                utterances.setProperty(i, "imagePath", backend.voicebanks[0].image_path || "")
            }
        }
        selectUtterance(selectedIndex)
    }

    function selectCombo(combo, value) {
        for (let i = 0; i < combo.count; ++i) {
            if (combo.valueAt(i) === value) {
                combo.currentIndex = i
                return
            }
        }
    }

    function selectUtterance(index) {
        if (index < 0 || index >= utterances.count)
            return
        selectedIndex = index
        const item = current()
        toneField.text = item.tone
        moraSpin.value = item.moraDuration
        pauseSpin.value = item.pauseDuration
        intonationSlider.value = item.intonation
        applyPitchCheck.checked = item.applyPitch
        pitchEditor.points = item.points || []
        pitchEditor.morae = item.morae || []
        selectCombo(voiceCombo, item.voicebankId)
        selectCombo(modelCombo, item.modelId)
        selectCombo(rendererCombo, item.renderer)
    }

    function addUtterance() {
        const voice = backend.voicebanks.length ? backend.voicebanks[0] : null
        utterances.append({
            utteranceId: "utterance-" + nextUtteranceId++,
            content: "", reading: "", morae: [], points: [],
            voicebankId: voice ? voice.id : "", imagePath: voice ? voice.image_path || "" : "",
            modelId: "none", renderer: "", tone: "C4",
            moraDuration: 120, pauseDuration: 180, intonation: 0, applyPitch: false
        })
        selectUtterance(utterances.count - 1)
        utteranceList.positionViewAtEnd()
    }

    function removeUtterance() {
        if (utterances.count <= 1)
            return
        utterances.remove(selectedIndex)
        selectedIndex = Math.min(selectedIndex, utterances.count - 1)
        selectUtterance(selectedIndex)
    }

    function moveUtterance(delta) {
        const target = selectedIndex + delta
        if (target < 0 || target >= utterances.count)
            return
        utterances.move(selectedIndex, target, 1)
        selectedIndex = target
        utteranceList.positionViewAtIndex(target, ListView.Contain)
    }

    function synthesizeCurrent() {
        const item = current()
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
        }
        if (item.applyPitch && item.points.some(value => Math.abs(value) > .1)) {
            request.manual_pitch = {
                version: 1,
                reading: item.reading,
                mode: "offset",
                points: item.points.map((cents, index) => ({position: index, cents: cents}))
            }
        }
        backend.synthesize(request)
    }
}
