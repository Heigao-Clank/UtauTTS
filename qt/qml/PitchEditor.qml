pragma ComponentBehavior: Bound

import QtQuick

Item {
    id: root
    property var points: []
    property var morae: []
    property var moraDurations: []
    property var moraPositions: []
    property int defaultMoraDuration: 120
    property int defaultPauseDuration: 180
    property int minimumMoraDuration: 20
    property int maximumMoraDuration: 1000
    property color accentColor: "#d35f6b"
    property color axisColor: "#c79298"
    property color gridColor: "#eadcdf"
    property color labelColor: "#66565a"
    property real moraWidth: 64
    property real sidePadding: 12
    signal pointsEdited(var points)
    signal moraDurationsEdited(var durations)
    signal moraPositionsEdited(var positions)
    property alias horizontalOffset: viewport.contentX
    readonly property real contentWidth: viewport.contentWidth
    readonly property real horizontalMaximum: Math.max(0, viewport.contentWidth - viewport.width)
    readonly property real horizontalVisibleRatio: viewport.contentWidth > 0 ? Math.min(1, viewport.width / viewport.contentWidth) : 1
    readonly property real horizontalPosition: horizontalMaximum > 0 ? viewport.contentX / horizontalMaximum : 0

    readonly property real durationScale: defaultMoraDuration > 0 ? moraWidth / defaultMoraDuration : 0.5
    readonly property real graphWidth: Math.max(width, sidePadding * 2 + root.totalDuration() * durationScale)

    function baseDurationAt(index) {
        const mora = index < root.morae.length ? root.morae[index] : null;
        const fallback = mora && mora.pause ? root.defaultPauseDuration : root.defaultMoraDuration;
        if (!mora || mora.pause)
            return Math.max(1, fallback);
        const values = root.moraDurations || [];
        const value = index < values.length ? Number(values[index]) : 0;
        if (!Number.isFinite(value) || value <= 0)
            return Math.max(1, fallback);
        return Math.max(root.minimumMoraDuration, Math.min(root.maximumMoraDuration, value));
    }

    function hasCompletePositions() {
        if (!root.morae.length || root.moraPositions.length < root.morae.length)
            return false;
        for (let index = 0; index < root.morae.length; ++index) {
            if (root.moraPositions[index] === null || root.moraPositions[index] === undefined)
                return false;
            if (!Number.isFinite(Number(root.moraPositions[index])))
                return false;
        }
        return true;
    }

    function positionAt(index) {
        if (index >= 0 && index < root.moraPositions.length) {
            const value = Number(root.moraPositions[index]);
            if (Number.isFinite(value))
                return Math.max(0, value);
        }
        return root.durationBefore(index) + root.baseDurationAt(index) / 2;
    }

    function durationAt(index) {
        if (!root.hasCompletePositions())
            return root.baseDurationAt(index);
        const position = root.positionAt(index);
        const left = index > 0
                     ? (root.positionAt(index - 1) + position) / 2
                     : Math.max(0, position - root.baseDurationAt(index) / 2);
        const right = index + 1 < root.morae.length
                      ? (position + root.positionAt(index + 1)) / 2
                      : position + root.baseDurationAt(index) / 2;
        return Math.max(root.minimumMoraDuration,
                        Math.min(root.maximumMoraDuration, right - left));
    }

    function totalDuration() {
        const count = Math.max(root.points.length, root.morae.length);
        let total = 0;
        for (let index = 0; index < count; ++index)
            total += root.baseDurationAt(index);
        if (root.hasCompletePositions() && root.morae.length) {
            const last = root.morae.length - 1;
            total = Math.max(total, root.positionAt(last) + root.baseDurationAt(last) / 2);
        }
        return total;
    }

    function durationBefore(index) {
        let total = 0;
        for (let position = 0; position < index; ++position)
            total += root.baseDurationAt(position);
        return total;
    }

    function durationIsEditable(index) {
        return index >= 0 && index < root.morae.length && !root.morae[index].pause;
    }

    function pointIsEditable(index) {
        if (index < 0 || index >= root.points.length)
            return false;
        return index >= root.morae.length || !root.morae[index].pause;
    }

    function pointX(index) {
        return root.sidePadding + root.positionAt(index) * root.durationScale;
    }

    function durationValuesFromPositions() {
        const values = [];
        for (let index = 0; index < root.morae.length; ++index)
            values.push(Math.round(root.durationAt(index)));
        return values;
    }

    function updatePositionAt(index, x, moveFollowing) {
        if (!root.durationIsEditable(index))
            return;
        const count = root.morae.length;
        const positions = (root.moraPositions || []).slice();
        for (let position = 0; position < count; ++position)
            positions[position] = root.positionAt(position);
        const minimumGap = root.minimumMoraDuration;
        const lower = index > 0 ? positions[index - 1] + minimumGap : 0;
        const upper = index + 1 < count
                      ? positions[index + 1] - minimumGap
                      : Math.max(root.totalDuration() + root.maximumMoraDuration / 2,
                                 positions[index] + minimumGap);
        const rawTarget = (x - root.sidePadding) / root.durationScale;
        const target = moveFollowing
                       ? Math.max(lower, rawTarget)
                       : Math.max(lower, Math.min(upper, rawTarget));
        const delta = target - positions[index];
        if (moveFollowing) {
            for (let position = index; position < count; ++position)
                positions[position] += delta;
        } else {
            positions[index] = target;
        }
        root.moraPositions = positions;

        root.moraDurations = root.durationValuesFromPositions();
        canvas.requestPaint();
    }

    function resetSingleDurationAt(index) {
        if (!root.durationIsEditable(index))
            return;
        const count = root.morae.length;
        const positions = (root.moraPositions || []).slice();
        for (let position = 0; position < count; ++position)
            positions[position] = root.positionAt(position);
        const currentDuration = root.durationAt(index);
        const targetDuration = root.defaultMoraDuration;
        const delta = targetDuration - currentDuration;
        if (Math.abs(delta) < 0.5)
            return;

        if (index + 1 < count) {
            const boundaryIndex = index + 1;
            const lower = positions[index] + root.minimumMoraDuration;
            const upper = boundaryIndex + 1 < count
                          ? positions[boundaryIndex + 1] - root.minimumMoraDuration
                          : Math.max(root.totalDuration() + root.maximumMoraDuration / 2,
                                     positions[boundaryIndex] + root.minimumMoraDuration);
            positions[boundaryIndex] = Math.max(lower,
                                                 Math.min(upper, positions[boundaryIndex] + delta * 2));
        } else {
            const lower = index > 0 ? positions[index - 1] + root.minimumMoraDuration : 0;
            const upper = Math.max(root.totalDuration() + root.maximumMoraDuration / 2,
                                  positions[index] + root.minimumMoraDuration);
            positions[index] = Math.max(lower,
                                        Math.min(upper, positions[index] + delta * 2));
        }
        root.moraPositions = positions;
    }

    function resetDurationAt(index) {
        if (!root.durationIsEditable(index))
            return;
        const first = Math.max(0, index - 1);
        const last = Math.min(root.morae.length - 1, index + 1);
        for (let target = first; target <= last; ++target)
            root.resetSingleDurationAt(target);
        root.moraDurations = root.durationValuesFromPositions();
        root.moraDurationsEdited(root.moraDurations.slice());
        root.moraPositionsEdited(root.moraPositions.slice());
        canvas.requestPaint();
    }

    function updatePitchAt(index, y) {
        if (index < 0 || index >= root.points.length || !root.pointIsEditable(index))
            return;
        const values = root.points.slice();
        const center = canvas.height / 2;
        const scale = Math.max(.05, Math.min(.36, canvas.height / 760));
        values[index] = Math.round(Math.max(-300, Math.min(300, (center - y) / scale)));
        root.points = values;
        canvas.requestPaint();
    }

    function resetPitchAt(index) {
        if (index < 0 || index >= root.points.length || !root.pointIsEditable(index))
            return;
        const values = root.points.slice();
        values[index] = 0;
        root.points = values;
        root.pointsEdited(values.slice());
        canvas.requestPaint();
    }

    function nearestEditablePoint(x) {
        let best = -1;
        let distance = Number.POSITIVE_INFINITY;
        for (let index = 0; index < root.points.length; ++index) {
            if (!root.pointIsEditable(index))
                continue;
            const candidateDistance = Math.abs(root.pointX(index) - x);
            if (candidateDistance < distance) {
                best = index;
                distance = candidateDistance;
            }
        }
        return best;
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
                anchors.bottomMargin: 64
                onWidthChanged: requestPaint()
                onHeightChanged: requestPaint()
                onPaint: {
                    const ctx = getContext("2d");
                    ctx.reset();
                    ctx.clearRect(0, 0, width, height);
                    const center = height / 2;
                    const scale = Math.max(.05, Math.min(.36, height / 760));
                    for (const cents of [-300, 0, 300]) {
                        ctx.strokeStyle = cents === 0 ? root.axisColor : root.gridColor;
                        ctx.setLineDash(cents === 0 ? [] : [4, 5]);
                        ctx.beginPath();
                        ctx.moveTo(0, center - cents * scale);
                        ctx.lineTo(width, center - cents * scale);
                        ctx.stroke();
                    }
                    ctx.setLineDash([]);
                    ctx.strokeStyle = root.axisColor;
                    ctx.globalAlpha = 0.45;
                    ctx.lineWidth = 1;
                    for (let index = 0; index < root.morae.length; ++index) {
                        if (!root.durationIsEditable(index))
                            continue;
                        const x = root.pointX(index);
                        ctx.beginPath();
                        ctx.moveTo(x, 0);
                        ctx.lineTo(x, height);
                        ctx.stroke();
                    }
                    ctx.globalAlpha = 1;
                    if (!root.points.length)
                        return;
                    ctx.strokeStyle = root.accentColor;
                    ctx.fillStyle = root.accentColor;
                    ctx.lineWidth = 2;
                    ctx.beginPath();
                    let started = false;
                    for (let index = 0; index < root.points.length; ++index) {
                        if (!root.pointIsEditable(index))
                            continue;
                        const x = root.pointX(index);
                        const y = center - root.points[index] * scale;
                        if (started)
                            ctx.lineTo(x, y);
                        else {
                            ctx.moveTo(x, y);
                            started = true;
                        }
                    }
                    ctx.stroke();
                    for (let index = 0; index < root.points.length; ++index) {
                        if (!root.pointIsEditable(index))
                            continue;
                        ctx.beginPath();
                        ctx.arc(root.pointX(index), center - root.points[index] * scale, 6, 0, Math.PI * 2);
                        ctx.fill();
                    }
                }
            }

            Item {
                width: root.graphWidth
                height: 64
                anchors.left: parent.left
                anchors.bottom: parent.bottom
                Repeater {
                    model: root.morae
                    delegate: Column {
                        id: pointColumn
                        required property var modelData
                        required property int index
                        width: root.moraWidth
                        height: parent.height
                        x: root.pointX(pointColumn.index) - width / 2
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
                            text: pointColumn.index < root.points.length ? Math.round(root.points[pointColumn.index]).toString() : "0"
                            horizontalAlignment: TextInput.AlignHCenter
                            color: root.labelColor
                            selectByMouse: true
                            validator: IntValidator {
                                bottom: -300
                                top: 300
                            }
                            onEditingFinished: {
                                const parsed = parseInt(text);
                                if (isNaN(parsed)) {
                                    text = Math.round(root.points[pointColumn.index]).toString();
                                    return;
                                }
                                const values = root.points.slice();
                                values[pointColumn.index] = Math.max(-300, Math.min(300, parsed));
                                root.points = values;
                                root.pointsEdited(values.slice());
                            }
                        }
                        Text {
                            width: parent.width
                            text: root.durationIsEditable(pointColumn.index)
                                  ? Math.round(root.durationAt(pointColumn.index)) + " ms" : ""
                            horizontalAlignment: Text.AlignHCenter
                            color: root.labelColor
                            opacity: 0.75
                            font.pixelSize: 10

                            MouseArea {
                                anchors.fill: parent
                                onDoubleClicked: root.resetDurationAt(pointColumn.index)
                            }
                        }
                    }
                }
            }

            Repeater {
                model: root.morae
                delegate: Item {
                    required property var modelData
                    required property int index
                    visible: root.durationIsEditable(index)
                    x: root.pointX(index) - width / 2
                    width: 14
                    height: parent.height - 64
                    z: 2

                    MouseArea {
                        anchors.fill: parent
                        property real pressX: 0
                        property real pressY: 0
                        property int dragMode: 0
                        property bool shiftFollowing: false
                        cursorShape: dragMode === 2 ? Qt.SizeVerCursor : Qt.SizeHorCursor
                        onPressed: mouse => {
                            const point = mapToItem(graph, mouse.x, mouse.y);
                            pressX = point.x;
                            pressY = point.y;
                            dragMode = 0;
                            shiftFollowing = false;
                        }
                        onPositionChanged: mouse => {
                            if (!pressed)
                                return;
                            const point = mapToItem(graph, mouse.x, mouse.y);
                            const deltaX = point.x - pressX;
                            const deltaY = point.y - pressY;
                            if (dragMode === 0 && Math.max(Math.abs(deltaX), Math.abs(deltaY)) >= 3) {
                                dragMode = Math.abs(deltaX) >= Math.abs(deltaY) ? 1 : 2;
                                shiftFollowing = (mouse.modifiers & Qt.ShiftModifier) !== 0;
                            }
                            if (dragMode === 1)
                                root.updatePositionAt(index, point.x, shiftFollowing);
                            else if (dragMode === 2)
                                root.updatePitchAt(index, mapToItem(canvas, mouse.x, mouse.y).y);
                        }
                        onReleased: {
                            if (dragMode === 1) {
                                root.moraDurationsEdited(root.moraDurations.slice());
                                root.moraPositionsEdited(root.moraPositions.slice());
                            } else if (dragMode === 2) {
                                root.pointsEdited(root.points.slice());
                            }
                            dragMode = 0;
                            shiftFollowing = false;
                        }
                        onCanceled: {
                            dragMode = 0;
                            shiftFollowing = false;
                        }
                        onDoubleClicked: root.resetPitchAt(index)
                    }
                }
            }

            /*
             * The duration handles sit on the pitch points.  The pitch editor
             * remains vertically draggable in the rest of the graph, while
             * dragging a handle changes that point's horizontal position.
             */
            /*
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
                            text: pointColumn.index < root.points.length ? Math.round(root.points[pointColumn.index]).toString() : "0"
                            horizontalAlignment: TextInput.AlignHCenter
                            color: root.labelColor
                            selectByMouse: true
                            validator: IntValidator {
                                bottom: -300
                                top: 300
                            }
                            onEditingFinished: {
                                const parsed = parseInt(text);
                                if (isNaN(parsed)) {
                                    text = Math.round(root.points[pointColumn.index]).toString();
                                    return;
                                }
                                const values = root.points.slice();
                                values[pointColumn.index] = Math.max(-300, Math.min(300, parsed));
                                root.points = values;
                                root.pointsEdited(values.slice());
                            }
                        }
                    }
                }
            }
            */

            MouseArea {
                anchors.fill: canvas
                property int dragging: -1
                onPressed: mouse => {
                    dragging = root.nearestEditablePoint(mouse.x);
                    if (dragging >= 0)
                        update(mouse.y);
                }
                onPositionChanged: mouse => {
                    if (dragging >= 0)
                        update(mouse.y);
                }
                onReleased: {
                    if (dragging >= 0)
                        root.pointsEdited(root.points.slice());
                    dragging = -1;
                }
                onDoubleClicked: mouse => {
                    const index = root.nearestEditablePoint(mouse.x);
                    if (index < 0)
                        return;
                    const values = root.points.slice();
                    values[index] = 0;
                    root.points = values;
                    root.pointsEdited(values.slice());
                    dragging = -1;
                }
                onCanceled: dragging = -1

                function update(y) {
                    root.updatePitchAt(dragging, y);
                }
            }
        }

        WheelHandler {
            acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
            onWheel: event => {
                const delta = event.angleDelta.y !== 0 ? event.angleDelta.y : event.angleDelta.x;
                viewport.contentX = Math.max(0, Math.min(viewport.contentWidth - viewport.width, viewport.contentX - delta));
                event.accepted = true;
            }
        }
    }

    onPointsChanged: canvas.requestPaint()
    onAccentColorChanged: canvas.requestPaint()
    onAxisColorChanged: canvas.requestPaint()
    onGridColorChanged: canvas.requestPaint()
}
