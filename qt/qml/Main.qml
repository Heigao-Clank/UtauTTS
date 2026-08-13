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
    width: 1240
    height: 800
    minimumWidth: 880
    minimumHeight: 600
    visible: true
    title: "UtauTTS"
    color: palette.window

    property color accent: palette.highlight
    property color borderColor: palette.mid
    property color mutedText: palette.placeholderText
    readonly property url repositoryUrl: "https://github.com/yh2237/UtauTTS"
    readonly property var appBackend: injectedBackend
    readonly property var licenseDocuments: injectedLegalDocuments
    property int selectedIndex: 0
    property int nextUtteranceId: 1
    property int draggedUtteranceIndex: -1

    ListModel { id: utterances }

    Timer {
        id: analyzeTimer
        interval: 250
        onTriggered: {
            if (utterances.count && window.current().content.trim()) {
                const item = window.current()
                window.appBackend.analyze(item.content, item.utteranceId)
            }
        }
    }

    MediaPlayer { id: player; audioOutput: AudioOutput {} }

    FileDialog {
        id: saveDialog
        fileMode: FileDialog.SaveFile
        nameFilters: ["WAV音声 (*.wav)"]
        defaultSuffix: "wav"
        onAccepted: window.appBackend.savePreview(selectedFile)
    }

    ApplicationWindow {
        id: settingsWindow
        title: "設定"
        visible: false
        width: 560
        height: 520
        minimumWidth: 440
        minimumHeight: 380
        transientParent: window
        modality: Qt.ApplicationModal
        flags: Qt.Dialog

        ScrollView {
            anchors.fill: parent
            contentWidth: availableWidth

            ColumnLayout {
                width: parent.width
                spacing: 14

                Label { text: "タイミング"; font.bold: true }
                GridLayout {
                    Layout.fillWidth: true
                    columns: 2
                    columnSpacing: 12
                    rowSpacing: 10

                    Label { text: "モーラ長" }
                    SpinBox {
                        id: moraSpin
                        Layout.fillWidth: true
                        from: 20
                        to: 1000
                        value: 120
                        editable: true
                        textFromValue: value => value + " ms"
                        valueFromText: text => parseInt(text)
                        onValueModified: window.updateSetting("moraDuration", value)
                    }
                    Label { text: "休止長" }
                    SpinBox {
                        id: pauseSpin
                        Layout.fillWidth: true
                        from: 0
                        to: 3000
                        value: 180
                        editable: true
                        textFromValue: value => value + " ms"
                        valueFromText: text => parseInt(text)
                        onValueModified: window.updateSetting("pauseDuration", value)
                    }
                }
                Label {
                    Layout.fillWidth: true
                    text: "モーラ長は通常の発音単位、休止長は句読点で挿入する無音の基準時間です。"
                    color: window.mutedText
                    wrapMode: Text.Wrap
                }

                Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: window.borderColor }

                Label { text: "ピッチ加工"; font.bold: true }
                Switch {
                    id: applyPitchCheck
                    text: "予測・手動ピッチを音声へ適用"
                    onToggled: window.updateSetting("applyPitch", checked)
                }
                Label {
                    Layout.fillWidth: true
                    text: "無効時は原音の声質と発音を優先します。有効時は選択したモデルと下部のピッチ編集をRendererへ渡します。"
                    color: window.mutedText
                    wrapMode: Text.Wrap
                }

                Item { Layout.fillHeight: true }
                Button {
                    text: "閉じる"
                    Layout.alignment: Qt.AlignRight
                    onClicked: settingsWindow.close()
                }
            }
        }
    }

    ApplicationWindow {
        id: aboutWindow
        title: "UtauTTSについて"
        visible: false
        width: 440
        height: 250
        transientParent: window
        modality: Qt.ApplicationModal
        flags: Qt.Dialog

        ColumnLayout {
            width: parent.width
            spacing: 12

            Label { text: "UtauTTS"; font.pixelSize: 24; font.bold: true }
            Label { text: "バージョン " + Qt.application.version }
            Label {
                Layout.fillWidth: true
                text: "UTAU音源を利用する音声合成ソフトウェア"
                color: window.mutedText
                wrapMode: Text.Wrap
            }
            RowLayout {
                Layout.fillWidth: true
                Button {
                    text: "GitHubリポジトリを開く"
                    onClicked: Qt.openUrlExternally(window.repositoryUrl)
                }
                Button {
                    text: "ライセンス"
                    onClicked: {
                        aboutWindow.close()
                        window.showAuxiliaryWindow(licenseWindow)
                    }
                }
                Item { Layout.fillWidth: true }
                Button { text: "閉じる"; onClicked: aboutWindow.close() }
            }
        }
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
                    text: window.licenseDocuments.length && licenseList.currentIndex >= 0
                          ? window.licenseDocuments[licenseList.currentIndex].text : ""
                    readOnly: true
                    selectByMouse: true
                    wrapMode: TextEdit.Wrap
                }
            }
        }
    }

    Connections {
        target: window.appBackend

        function onMetadataChanged() {
            window.assignDefaultVoicebank()
            window.assignDefaultSynthesisSettings()
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

    menuBar: MenuBar {
        Menu {
            title: "ファイル"
            MenuItem {
                text: "WAVを保存..."
                enabled: player.source.toString().length > 0
                onTriggered: saveDialog.open()
            }
            MenuItem {
                text: "音源を再読込"
                enabled: !window.appBackend.busy
                onTriggered: window.appBackend.reloadVoicebanks()
            }
            MenuSeparator {}
            MenuItem { text: "終了"; onTriggered: Qt.quit() }
        }
        Menu {
            title: "設定"
            MenuItem { text: "設定..."; onTriggered: window.showAuxiliaryWindow(settingsWindow) }
        }
        Menu {
            title: "ヘルプ"
            MenuItem { text: "UtauTTSについて..."; onTriggered: window.showAuxiliaryWindow(aboutWindow) }
            MenuItem { text: "GitHubリポジトリ"; onTriggered: Qt.openUrlExternally(window.repositoryUrl) }
            MenuItem { text: "ライセンス..."; onTriggered: window.showAuxiliaryWindow(licenseWindow) }
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
                background: Rectangle { color: window.palette.window }

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
                                border.color: card.index === window.selectedIndex
                                              ? window.accent : window.borderColor

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
                                            window.selectUtterance(card.index)
                                            window.draggedUtteranceIndex = card.index
                                            dragProxy.x = imageHandle.x
                                            dragProxy.y = imageHandle.y
                                        } else window.draggedUtteranceIndex = -1
                                    }
                                }
                                ToolTip.visible: imageHover.hovered && !imageDrag.active
                                ToolTip.text: window.voicebankName(card.voicebankId) + "\nドラッグして並べ替え"
                                HoverHandler { id: imageHover }
                            }

                            TextField {
                                id: utteranceEditor
                                Layout.fillWidth: true
                                Layout.preferredHeight: 42
                                text: card.content
                                font.pixelSize: 16
                                placeholderText: "読み上げる文章を入力"
                                selectByMouse: true

                                onActiveFocusChanged: if (activeFocus) window.selectUtterance(card.index)
                                onTextChanged: {
                                    if (card.index >= utterances.count || utterances.get(card.index).content === text)
                                        return
                                    utterances.setProperty(card.index, "content", text)
                                    window.selectUtterance(card.index)
                                    analyzeTimer.restart()
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
                                            window.selectUtterance(card.index)
                                            window.moveUtterance(-1)
                                        }
                                    }
                                    MenuItem {
                                        text: "下へ移動"
                                        enabled: card.index < utterances.count - 1
                                        onTriggered: {
                                            window.selectUtterance(card.index)
                                            window.moveUtterance(1)
                                        }
                                    }
                                    MenuSeparator {}
                                    MenuItem {
                                        text: "削除"
                                        enabled: utterances.count > 1
                                        onTriggered: {
                                            window.selectUtterance(card.index)
                                            window.removeUtterance()
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
                                if (!drag.source) return
                                const from = window.draggedUtteranceIndex
                                const to = card.index
                                if (from < 0 || to < 0 || from === to) return
                                utterances.move(from, to, 1)
                                window.selectedIndex = to
                                window.draggedUtteranceIndex = to
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
                    ToolTip.text: "発話を追加"
                }
            }

            Pane {
                SplitView.preferredWidth: 268
                SplitView.minimumWidth: 238
                SplitView.maximumWidth: 340
                padding: 14
                background: Rectangle { color: window.palette.window; border.color: window.borderColor }

                ScrollView {
                    anchors.fill: parent
                    contentWidth: availableWidth

                    ColumnLayout {
                        width: parent.width
                        spacing: 12

                        Label { text: "音源"; font.pixelSize: 12; color: window.mutedText }
                        ComboBox {
                            id: voiceCombo
                            Layout.fillWidth: true
                            model: window.appBackend.voicebanks
                            textRole: "name"
                            valueRole: "id"
                            onActivated: {
                                window.updateSetting("voicebankId", currentValue)
                                const voice = window.voicebankById(currentValue)
                                window.updateSetting("imagePath", voice ? voice.image_path : "")
                            }
                        }

                        Label { text: "抑揚モデル"; font.pixelSize: 12; color: window.mutedText }
                        ComboBox {
                            id: modelCombo
                            Layout.fillWidth: true
                            model: [{id: "none", display_name: "なし"}].concat(window.appBackend.models)
                            textRole: "display_name"
                            valueRole: "id"
                            onActivated: {
                                window.updateSetting("modelId", currentValue)
                                const model = window.modelById(currentValue)
                                const renderer = window.preferredRendererForModel(model)
                                if (renderer) {
                                    window.updateSetting("renderer", renderer)
                                    window.selectCombo(rendererCombo, renderer)
                                }
                            }
                        }

                        Label { text: "Renderer"; font.pixelSize: 12; color: window.mutedText }
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
                            text: window.appBackend.cudaAvailable
                                  ? "CUDA GPUを検出しました" : "CPUモード"
                            color: window.mutedText
                            font.pixelSize: 11
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Label { text: "音高"; Layout.fillWidth: true }
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
                            Label { text: "抑揚"; Layout.fillWidth: true }
                            Label { text: intonationSlider.value.toFixed(2); color: window.mutedText }
                        }
                        Slider {
                            id: intonationSlider
                            Layout.fillWidth: true
                            from: 0
                            to: 1
                            stepSize: .05
                            onMoved: window.updateSetting("intonation", value)
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Label { text: "モーラ長"; Layout.fillWidth: true }
                            Label { text: Math.round(moraSlider.value) + " ms"; color: window.mutedText }
                        }
                        Slider {
                            id: moraSlider
                            Layout.fillWidth: true
                            from: 60
                            to: 300
                            stepSize: 5
                            onMoved: {
                                window.updateSetting("moraDuration", value)
                                moraSpin.value = value
                            }
                        }

                        RowLayout {
                            Layout.fillWidth: true
                            Label { text: "休止長"; Layout.fillWidth: true }
                            Label { text: Math.round(pauseSlider.value) + " ms"; color: window.mutedText }
                        }
                        Slider {
                            id: pauseSlider
                            Layout.fillWidth: true
                            from: 0
                            to: 800
                            stepSize: 10
                            onMoved: {
                                window.updateSetting("pauseDuration", value)
                                pauseSpin.value = value
                            }
                        }

                        Item { Layout.fillHeight: true }
                    }
                }
            }
        }

        Pane {
            SplitView.preferredHeight: 238
            SplitView.minimumHeight: 150
            padding: 0
            background: Rectangle { color: window.palette.window; border.color: window.borderColor }

            ColumnLayout {
                anchors.fill: parent
                spacing: 0

                RowLayout {
                    Layout.fillWidth: true
                    Layout.preferredHeight: 38
                    Layout.leftMargin: 12
                    Layout.rightMargin: 10
                    Label { text: "イントネーション"; font.pixelSize: 12 }
                    Item { Layout.fillWidth: true }
                    Label { text: "±300 cent"; color: window.mutedText; font.pixelSize: 11 }
                }
                Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 1; color: window.borderColor }
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
                    onPointsEdited: points => utterances.setProperty(window.selectedIndex, "points", points)
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
                        text: window.appBackend.busy ? "…"
                              : player.playbackState === MediaPlayer.PlayingState ? "Ⅱ" : "▶"
                        highlighted: true
                        enabled: player.playbackState === MediaPlayer.PlayingState
                                 || (!window.appBackend.busy && utterances.count
                                     && window.current().content.trim().length > 0)
                        onClicked: {
                            if (player.playbackState === MediaPlayer.PlayingState)
                                player.pause()
                            else if (player.playbackState === MediaPlayer.PausedState)
                                player.play()
                            else
                                window.synthesizeCurrent()
                        }
                        ToolTip.visible: hovered
                        ToolTip.text: window.appBackend.error.length
                                      ? window.appBackend.error
                                      : player.playbackState === MediaPlayer.PlayingState
                                        ? "一時停止" : "生成して再生"
                    }
                    Slider {
                        Layout.fillWidth: true
                        from: 0
                        to: Math.max(1, player.duration)
                        value: player.position
                        enabled: player.source.toString().length > 0
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

    function current() { return utterances.get(selectedIndex) }

    function showAuxiliaryWindow(auxiliaryWindow) {
        auxiliaryWindow.show()
        auxiliaryWindow.raise()
        auxiliaryWindow.requestActivate()
    }

    function voicebankById(id) {
        for (let i = 0; i < window.appBackend.voicebanks.length; ++i)
            if (window.appBackend.voicebanks[i].id === id) return window.appBackend.voicebanks[i]
        return null
    }

    function modelById(id) {
        for (let i = 0; i < window.appBackend.models.length; ++i)
            if (window.appBackend.models[i].id === id) return window.appBackend.models[i]
        return null
    }

    function rendererById(id) {
        for (let i = 0; i < window.appBackend.renderers.length; ++i)
            if (window.appBackend.renderers[i].id === id) return window.appBackend.renderers[i]
        return null
    }

    function modelDescription(id) {
        if (!id || id === "none") return "モデルを使わず、原音のピッチを維持します。"
        const model = modelById(id)
        return model ? model.description || "" : ""
    }

    function rendererDescription(id) {
        if (!id) return "既定Rendererはplugin manifestの優先度から選択されます。"
        const renderer = rendererById(id)
        return renderer ? renderer.description || "" : ""
    }

    function defaultModelId() {
        return window.appBackend.models.length ? window.appBackend.models[0].id : "none"
    }

    function preferredRendererForModel(model) {
        const preferredAcceleration = window.appBackend.cudaAvailable ? "cuda" : "cpu"
        const recommended = model && model.recommended_renderers ? model.recommended_renderers : []
        for (let pass = 0; pass < 2; ++pass) {
            for (let index = 0; index < recommended.length; ++index) {
                const renderer = window.rendererById(recommended[index])
                if (renderer && (pass === 1 || renderer.acceleration === preferredAcceleration))
                    return renderer.id
            }
        }
        for (let index = 0; index < window.appBackend.renderers.length; ++index) {
            const renderer = window.appBackend.renderers[index]
            if (renderer.acceleration === preferredAcceleration)
                return renderer.id
        }
        return window.appBackend.defaultRenderer
    }

    function defaultRendererId() {
        return window.preferredRendererForModel(window.modelById(window.defaultModelId()))
    }

    function utteranceIndex(id) {
        for (let i = 0; i < utterances.count; ++i)
            if (utterances.get(i).utteranceId === id) return i
        return -1
    }

    function voicebankName(id) {
        const voice = voicebankById(id)
        return voice ? voice.name : "音源未選択"
    }

    function localImageUrl(path) {
        return path ? encodeURI("file:///" + path.replace(/\\/g, "/")) : ""
    }

    function formatTime(milliseconds) {
        const seconds = Math.max(0, Math.floor(milliseconds / 1000))
        return Math.floor(seconds / 60) + ":" + String(seconds % 60).padStart(2, "0")
    }

    function updateSetting(name, value) {
        if (utterances.count) utterances.setProperty(selectedIndex, name, value)
    }

    function assignDefaultVoicebank() {
        if (!utterances.count || !window.appBackend.voicebanks.length) return
        for (let i = 0; i < utterances.count; ++i) {
            const item = utterances.get(i)
            if (!item.voicebankId) {
                utterances.setProperty(i, "voicebankId", window.appBackend.voicebanks[0].id)
                utterances.setProperty(i, "imagePath", window.appBackend.voicebanks[0].image_path || "")
            }
        }
        selectUtterance(selectedIndex)
    }

    function assignDefaultSynthesisSettings() {
        if (!utterances.count || !window.appBackend.models.length || !window.appBackend.renderers.length)
            return
        const modelId = window.defaultModelId()
        const rendererId = window.defaultRendererId()
        for (let index = 0; index < utterances.count; ++index) {
            const item = utterances.get(index)
            if (!item.modelId) utterances.setProperty(index, "modelId", modelId)
            if (!item.renderer) utterances.setProperty(index, "renderer", rendererId)
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
        if (index < 0 || index >= utterances.count) return
        selectedIndex = index
        const item = current()
        toneField.text = item.tone
        moraSlider.value = item.moraDuration
        pauseSlider.value = item.pauseDuration
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
        const voice = window.appBackend.voicebanks.length ? window.appBackend.voicebanks[0] : null
        utterances.append({
            utteranceId: "utterance-" + nextUtteranceId++,
            content: "", reading: "", morae: [], points: [],
            voicebankId: voice ? voice.id : "", imagePath: voice ? voice.image_path || "" : "",
            modelId: window.appBackend.models.length ? window.defaultModelId() : "",
            renderer: window.appBackend.renderers.length ? window.defaultRendererId() : "",
            tone: "C4", moraDuration: 120, pauseDuration: 180,
            intonation: 0, applyPitch: true
        })
        selectUtterance(utterances.count - 1)
        utteranceList.positionViewAtEnd()
    }

    function removeUtterance() {
        if (utterances.count <= 1) return
        utterances.remove(selectedIndex)
        selectedIndex = Math.min(selectedIndex, utterances.count - 1)
        selectUtterance(selectedIndex)
    }

    function moveUtterance(delta) {
        const target = selectedIndex + delta
        if (target < 0 || target >= utterances.count) return
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
        window.appBackend.synthesize(request)
    }
}
