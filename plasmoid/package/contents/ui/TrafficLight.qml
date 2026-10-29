import QtQuick

// A small traffic light: three lamps, the pattern comes from the hub's
// "light" field (red, green_blink, red_yellow_alternate, …).
Rectangle {
    id: housing

    property string light: "off"
    property bool blinkOn: true // toggled by the parent twice a second

    readonly property real lampSize: Math.max(2, Math.min(width * 0.8, (height - spacing * 4) / 3))
    readonly property real spacing: Math.max(1, height * 0.04)

    radius: width * 0.25
    color: "#24262c"
    implicitWidth: 18
    implicitHeight: 44

    function lit(lamp) {
        switch (light) {
        case "red": return lamp === "red"
        case "yellow": return lamp === "yellow"
        case "green": return lamp === "green"
        case "green_blink": return lamp === "green" && blinkOn
        case "red_yellow": return lamp === "red" || lamp === "yellow"
        case "red_yellow_alternate": return (lamp === "red" && blinkOn) || (lamp === "yellow" && !blinkOn)
        case "yellow_blink": return lamp === "yellow" && blinkOn
        }
        return false
    }

    Column {
        anchors.centerIn: parent
        spacing: housing.spacing
        Repeater {
            model: [["red", "#e5322d"], ["yellow", "#f5b301"], ["green", "#1fa84f"]]
            Rectangle {
                width: housing.lampSize
                height: width
                radius: width / 2
                color: modelData[1]
                opacity: housing.lit(modelData[0]) ? 1 : 0.22
            }
        }
    }
}
