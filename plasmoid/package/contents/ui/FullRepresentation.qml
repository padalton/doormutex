import QtQuick
import QtQuick.Layouts
import QtQuick.Controls as QQC2
import org.kde.plasma.components as PlasmaComponents3
import org.kde.plasma.extras as PlasmaExtras
import org.kde.kirigami as Kirigami

// The popup: state, override with a duration, pomodoro, message and
// today's plan as a bar.
PlasmaExtras.Representation {
    id: full

    readonly property var st: root.hubState
    property var slots: []

    Layout.minimumWidth: Kirigami.Units.gridUnit * 20
    Layout.minimumHeight: Kirigami.Units.gridUnit * 22
    collapseMarginsHint: true

    function loadDay() {
        const from = new Date()
        from.setHours(0, 0, 0, 0)
        const to = new Date(from.getTime() + 24 * 3600 * 1000)
        root.request("GET", "/api/v1/timeline?from=" + encodeURIComponent(from.toISOString()) +
            "&to=" + encodeURIComponent(to.toISOString()), null, function (data) {
            if (data && data.slots) full.slots = data.slots
        })
    }

    Connections {
        target: root
        function onExpandedChanged() {
            if (root.expanded) full.loadDay()
        }
    }

    readonly property var colors: ({ "dnd": "#e5322d", "working": "#f5b301", "free": "#1fa84f", "away": "#3a3d44" })

    ColumnLayout {
        anchors.fill: parent
        anchors.margins: Kirigami.Units.largeSpacing
        spacing: Kirigami.Units.mediumSpacing

        RowLayout {
            spacing: Kirigami.Units.largeSpacing
            TrafficLight {
                Layout.preferredWidth: Kirigami.Units.gridUnit * 2.2
                Layout.preferredHeight: Kirigami.Units.gridUnit * 5.5
                light: full.st ? full.st.light : "off"
                blinkOn: root.blinkOn
            }
            ColumnLayout {
                Layout.fillWidth: true
                spacing: 0
                Kirigami.Heading {
                    level: 2
                    text: root.error !== "" ? root.error
                        : !full.st ? i18n("Connecting…")
                        : full.st.phase === "fault" ? i18n("Fault – companion not responding")
                        : root.statusText[full.st.status]
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }
                PlasmaComponents3.Label {
                    visible: !!full.st
                    text: full.st ? (full.st.until ? i18n("until %1", root.time(full.st.until)) : i18n("until further notice")) : ""
                }
                PlasmaComponents3.Label {
                    visible: !!full.st
                    text: root.describe(full.st)
                    opacity: 0.7
                    wrapMode: Text.Wrap
                    Layout.fillWidth: true
                }
                PlasmaComponents3.Label {
                    visible: !!(full.st && full.st.next_free && full.st.status !== "free")
                    text: full.st && full.st.next_free ? i18n("free from %1", root.time(full.st.next_free.start)) : ""
                    opacity: 0.7
                }
                PlasmaComponents3.Label {
                    visible: !!(full.st && full.st.message)
                    text: full.st && full.st.message ? "“" + full.st.message + "”" : ""
                    font.italic: true
                }
            }
        }

        // Today: one coloured bar, proportional to time.
        Item {
            Layout.fillWidth: true
            Layout.preferredHeight: Kirigami.Units.gridUnit
            Row {
                anchors.fill: parent
                Repeater {
                    model: full.slots
                    Rectangle {
                        readonly property real from: new Date(new Date().setHours(0, 0, 0, 0)).getTime()
                        readonly property real a: Math.max(new Date(modelData.since).getTime(), from)
                        readonly property real b: Math.min(new Date(modelData.until).getTime(), from + 24 * 3600 * 1000)
                        width: parent.width * Math.max(0, b - a) / (24 * 3600 * 1000)
                        height: parent.height
                        color: full.colors[modelData.status] || "transparent"
                    }
                }
            }
        }

        Kirigami.Separator { Layout.fillWidth: true }

        RowLayout {
            PlasmaComponents3.Label { text: i18n("Override for") }
            PlasmaComponents3.ComboBox {
                id: duration
                readonly property var values: ["15m", "30m", "1h", "2h", "end_of_event", "end_of_workday", "next_change", "indefinite"]
                readonly property string choice: values[currentIndex]
                Layout.fillWidth: true
                currentIndex: 2
                model: [
                    i18n("15 minutes"), i18n("30 minutes"), i18n("1 hour"), i18n("2 hours"),
                    i18n("until the event ends"), i18n("until end of work day"),
                    i18n("until the next automatic change"), i18n("until changed")
                ]
            }
        }
        GridLayout {
            columns: 5
            Layout.fillWidth: true
            Repeater {
                model: [["dnd", i18n("Red")], ["working", i18n("Yellow")], ["free", i18n("Green")], ["away", i18n("Off")]]
                PlasmaComponents3.Button {
                    Layout.fillWidth: true
                    text: modelData[1]
                    icon.name: ({ "dnd": "user-busy", "working": "user-away", "free": "user-available", "away": "user-offline" })[modelData[0]]
                    onClicked: {
                        const v = duration.choice
                        const mode = /^[0-9]/.test(v) ? "duration" : v
                        root.setOverride(modelData[0], mode, v)
                    }
                }
            }
            PlasmaComponents3.Button {
                Layout.fillWidth: true
                text: i18n("Auto")
                icon.name: "view-refresh"
                checkable: false
                highlighted: !!(full.st && full.st.override)
                onClicked: root.clearOverride()
            }
        }

        RowLayout {
            PlasmaComponents3.Button {
                text: "🍅 25 / 5"
                onClicked: root.startPomodoro("25m", "5m")
            }
            PlasmaComponents3.Button {
                text: "🍅 50 / 10"
                onClicked: root.startPomodoro("50m", "10m")
            }
            PlasmaComponents3.Button {
                text: i18n("Stop")
                enabled: !!(full.st && full.st.pomodoro)
                onClicked: root.stopPomodoro()
            }
        }

        RowLayout {
            PlasmaComponents3.TextField {
                id: message
                Layout.fillWidth: true
                maximumLength: 64
                placeholderText: i18n("Message for the e-ink")
                onAccepted: root.setMessage(text)
            }
            PlasmaComponents3.Button {
                icon.name: "edit-clear"
                onClicked: { message.text = ""; root.setMessage("") }
                QQC2.ToolTip.text: i18n("Clear message")
                QQC2.ToolTip.visible: hovered
            }
        }

        Item { Layout.fillHeight: true }

        RowLayout {
            PlasmaComponents3.Label {
                text: root.live ? i18n("● live") : i18n("○ polling")
                opacity: 0.6
                font: Kirigami.Theme.smallFont
            }
            Item { Layout.fillWidth: true }
            PlasmaComponents3.ToolButton {
                text: i18n("Web UI")
                icon.name: "internet-web-browser"
                onClicked: Qt.openUrlExternally(root.hub + "/")
            }
        }
    }
}
