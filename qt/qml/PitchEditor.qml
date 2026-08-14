pragma ComponentBehavior: Bound

import QtQuick

Item {
    id: root
    property var points: []
    property var morae: []
    property color accentColor: "#58a875"
    property color axisColor: "#8fbc9c"
    property color gridColor: "#d8e4db"
    property color labelColor: "#59665e"
    property real moraWidth: 64
    property real sidePadding: 12
    signal pointsEdited(var points)
    property alias horizontalOffset: viewport.contentX
    readonly property real contentWidth: viewport.contentWidth
    readonly property real horizontalMaximum: Math.max(0, viewport.contentWidth - viewport.width)
    readonly property real horizontalVisibleRatio: viewport.contentWidth > 0
                                                ? Math.min(1, viewport.width / viewport.contentWidth) : 1
    readonly property real horizontalPosition: horizontalMaximum > 0
                                            ? viewport.contentX / horizontalMaximum : 0

    readonly property real graphWidth: Math.max(width,
                                                sidePadding * 2 + Math.max(points.length, morae.length) * moraWidth)

    function pointIsEditable(index) {
        if (index < 0 || index >= root.points.length) return false
        return index >= root.morae.length || !root.morae[index].pause
    }

    function pointX(index) {
        return root.sidePadding + index * root.moraWidth + root.moraWidth / 2
    }

    function nearestEditablePoint(x) {
        let best = -1
        let distance = Number.POSITIVE_INFINITY
        for (let index = 0; index < root.points.length; ++index) {
            if (!root.pointIsEditable(index)) continue
            const candidateDistance = Math.abs(root.pointX(index) - x)
            if (candidateDistance < distance) {
                best = index
                distance = candidateDistance
            }
        }
        return best
    }

    Flickable {
        id: viewport
        anchors.fill: parent
        clip: true
        contentWidth: root.graphWidth
        contentHeight: height
        boundsBehavior: Flickable.StopAtBounds
        interactive: false

        Item {
            id: graph
            width: root.graphWidth
            height: viewport.height

            Canvas {
                id: canvas
                anchors.fill: parent
                anchors.bottomMargin: 52
                onWidthChanged: requestPaint()
                onHeightChanged: requestPaint()
                onPaint: {
                    const ctx = getContext("2d")
                    ctx.reset()
                    ctx.clearRect(0, 0, width, height)
                    const center = height / 2
                    const scale = Math.max(.05, Math.min(.36, height / 760))
                    for (const cents of [-300, 0, 300]) {
                        ctx.strokeStyle = cents === 0 ? root.axisColor : root.gridColor
                        ctx.setLineDash(cents === 0 ? [] : [4, 5])
                        ctx.beginPath()
                        ctx.moveTo(0, center - cents * scale)
                        ctx.lineTo(width, center - cents * scale)
                        ctx.stroke()
                    }
                    ctx.setLineDash([])
                    if (!root.points.length) return
                    ctx.strokeStyle = root.accentColor
                    ctx.fillStyle = root.accentColor
                    ctx.lineWidth = 2
                    ctx.beginPath()
                    let started = false
                    for (let index = 0; index < root.points.length; ++index) {
                        if (!root.pointIsEditable(index)) continue
                        const x = root.pointX(index)
                        const y = center - root.points[index] * scale
                        if (started) ctx.lineTo(x, y)
                        else {
                            ctx.moveTo(x, y)
                            started = true
                        }
                    }
                    ctx.stroke()
                    for (let index = 0; index < root.points.length; ++index) {
                        if (!root.pointIsEditable(index)) continue
                        ctx.beginPath()
                        ctx.arc(root.pointX(index), center - root.points[index] * scale,
                                6, 0, Math.PI * 2)
                        ctx.fill()
                    }
                }
            }

            Row {
                anchors.left: parent.left
                anchors.leftMargin: root.sidePadding
                anchors.bottom: parent.bottom
                height: 50

                Repeater {
                    model: root.morae
                    delegate: Column {
                        id: pointColumn
                        required property var modelData
                        required property int index
                        width: root.moraWidth
                        spacing: 1

                        Text {
                            width: parent.width
                            text: pointColumn.modelData.mora || "・"
                            horizontalAlignment: Text.AlignHCenter
                            color: root.labelColor
                            elide: Text.ElideRight
                        }
                        TextInput {
                            width: parent.width - 8
                            height: 24
                            anchors.horizontalCenter: parent.horizontalCenter
                            visible: root.pointIsEditable(pointColumn.index)
                            text: pointColumn.index < root.points.length
                                  ? Math.round(root.points[pointColumn.index]).toString() : "0"
                            horizontalAlignment: TextInput.AlignHCenter
                            color: root.labelColor
                            selectByMouse: true
                            validator: IntValidator { bottom: -300; top: 300 }
                            onEditingFinished: {
                                const parsed = parseInt(text)
                                if (isNaN(parsed)) {
                                    text = Math.round(root.points[pointColumn.index]).toString()
                                    return
                                }
                                const values = root.points.slice()
                                values[pointColumn.index] = Math.max(-300, Math.min(300, parsed))
                                root.points = values
                                root.pointsEdited(values.slice())
                            }
                        }
                    }
                }
            }

            MouseArea {
                anchors.fill: canvas
                property int dragging: -1
                onPressed: mouse => {
                    dragging = root.nearestEditablePoint(mouse.x)
                    if (dragging >= 0) update(mouse.y)
                }
                onPositionChanged: mouse => {
                    if (dragging >= 0) update(mouse.y)
                }
                onReleased: {
                    if (dragging >= 0) root.pointsEdited(root.points.slice())
                    dragging = -1
                }
                onDoubleClicked: mouse => {
                    const index = root.nearestEditablePoint(mouse.x)
                    if (index < 0) return
                    const values = root.points.slice()
                    values[index] = 0
                    root.points = values
                    root.pointsEdited(values.slice())
                    dragging = -1
                }
                onCanceled: dragging = -1

                function update(y) {
                    const values = root.points.slice()
                    const center = canvas.height / 2
                    const scale = Math.max(.05, Math.min(.36, canvas.height / 760))
                    values[dragging] = Math.round(Math.max(-300, Math.min(300, (center - y) / scale)))
                    root.points = values
                    canvas.requestPaint()
                }
            }
        }

        WheelHandler {
            acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
            onWheel: event => {
                const delta = event.angleDelta.y !== 0 ? event.angleDelta.y : event.angleDelta.x
                viewport.contentX = Math.max(0, Math.min(viewport.contentWidth - viewport.width,
                                                         viewport.contentX - delta))
                event.accepted = true
            }
        }
    }

    onPointsChanged: canvas.requestPaint()
    onAccentColorChanged: canvas.requestPaint()
    onAxisColorChanged: canvas.requestPaint()
    onGridColorChanged: canvas.requestPaint()
}
