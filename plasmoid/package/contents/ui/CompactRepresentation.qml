import QtQuick
import QtQuick.Layouts
import org.kde.plasma.plasmoid

// The panel icon: a mini traffic light. Click opens the popup.
MouseArea {
    id: compact

    Layout.minimumWidth: Plasmoid.formFactor === 2 /* vertical panel */ ? parent.width : height * 0.45
    hoverEnabled: true
    onClicked: root.expanded = !root.expanded

    TrafficLight {
        anchors.centerIn: parent
        height: parent.height * 0.92
        width: height * 0.42
        light: root.hubState ? root.hubState.light : "off"
        blinkOn: root.blinkOn
        opacity: root.error === "" ? 1 : 0.5
    }
}
