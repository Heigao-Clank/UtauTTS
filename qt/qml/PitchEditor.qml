import QtQuick

Item {
    id: root
    property var points: []
    property var morae: []
    signal pointsEdited(var points)

    Canvas {
        id: canvas
        anchors.fill: parent
        anchors.bottomMargin: 30
        onWidthChanged: requestPaint()
        onHeightChanged: requestPaint()
        onPaint: {
            const ctx=getContext("2d");ctx.reset();ctx.clearRect(0,0,width,height)
            const center=height/2,scale=Math.min(.36,height/760)
            for(const cents of [-300,0,300]){ctx.strokeStyle=cents===0?"#8fbc9c":"#d8e4db";ctx.setLineDash(cents===0?[]:[4,5]);ctx.beginPath();ctx.moveTo(0,center-cents*scale);ctx.lineTo(width,center-cents*scale);ctx.stroke()}
            ctx.setLineDash([]);if(!root.points.length)return;ctx.strokeStyle="#58a875";ctx.fillStyle="#58a875";ctx.lineWidth=2;ctx.beginPath()
            for(let i=0;i<root.points.length;i++){const x=root.points.length===1?width/2:16+i*(width-32)/(root.points.length-1),y=center-root.points[i]*scale;i?ctx.lineTo(x,y):ctx.moveTo(x,y)}ctx.stroke()
            for(let i=0;i<root.points.length;i++){const x=root.points.length===1?width/2:16+i*(width-32)/(root.points.length-1),y=center-root.points[i]*scale;ctx.beginPath();ctx.arc(x,y,6,0,Math.PI*2);ctx.fill()}
        }
    }
    Row {
        anchors.left: canvas.left;anchors.right: canvas.right;anchors.bottom: parent.bottom;height:26
        Repeater { model: root.morae; delegate: Text { required property var modelData;width:root.morae.length?canvas.width/root.morae.length:0;text:modelData.mora||"・";horizontalAlignment:Text.AlignHCenter;color:"#59665e";elide:Text.ElideRight } }
    }
    MouseArea {
        anchors.fill: canvas
        property int dragging: -1
        onPressed: mouse=>{if(!root.points.length)return;dragging=root.points.length===1?0:Math.max(0,Math.min(root.points.length-1,Math.round((mouse.x-16)*(root.points.length-1)/(width-32))));update(mouse.y)}
        onPositionChanged: mouse=>{if(dragging>=0)update(mouse.y)}
        onReleased: {dragging=-1;root.pointsEdited(root.points.slice())}
        function update(y){const values=root.points.slice(),center=canvas.height/2,scale=Math.min(.36,canvas.height/760);values[dragging]=Math.round(Math.max(-300,Math.min(300,(center-y)/scale)));root.points=values;canvas.requestPaint()}
    }
    onPointsChanged: canvas.requestPaint()
}
