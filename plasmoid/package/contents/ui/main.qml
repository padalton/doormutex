import QtQuick
import org.kde.plasma.plasmoid
import org.kde.plasma.core as PlasmaCore

// doormutex widget: talks to the hub's REST API and follows its event
// stream (Server-Sent Events); polls as a fallback.
PlasmoidItem {
    id: root

    property var hubState: null
    property string error: ""
    property bool live: false
    property bool blinkOn: true

    readonly property string hub: String(Plasmoid.configuration.hubUrl).replace(/\/+$/, "")
    readonly property string token: Plasmoid.configuration.token
    readonly property string profile: Plasmoid.configuration.profile
    readonly property string pomodoroWorkStatus: Plasmoid.configuration.pomodoroWorkStatus

    readonly property var statusText: ({
        "dnd": i18n("Do not disturb"), "working": i18n("Working"),
        "free": i18n("Come in"), "away": i18n("Not here")
    })
    readonly property var reasonText: ({
        "override": i18n("set by hand"), "pomodoro": i18n("pomodoro"), "pomodoro_break": i18n("pomodoro break"),
        "microphone": i18n("microphone in use"), "camera": i18n("camera in use"),
        "screen_locked": i18n("screen locked"), "pc_off": i18n("PC off"),
        "companion_lost": i18n("companion not responding"), "companion_offline": i18n("PC not running"),
        "calendar": i18n("calendar"), "work_hours": i18n("working hours"), "signal": i18n("signal"),
        "default": i18n("nothing planned")
    })

    function describe(st) {
        if (!st) return ""
        let r = reasonText[st.reason] || st.reason
        if (st.detail) r += " – " + String(st.detail).replace(/_/g, " ")
        return r
    }

    function time(iso) {
        if (!iso) return ""
        const d = new Date(iso)
        const today = new Date()
        return d.toDateString() === today.toDateString()
            // Not Qt.formatTime(d, Locale.ShortFormat): that enum is read as Qt.ISODate (HH:mm:ss).
            ? Qt.formatTime(d, Qt.locale().timeFormat(Locale.ShortFormat))
            : Qt.formatDateTime(d, "ddd " + Qt.locale().timeFormat(Locale.ShortFormat))
    }

    function url(path) {
        let u = hub + path
        if (profile !== "") u += (u.indexOf("?") < 0 ? "?" : "&") + "profile=" + encodeURIComponent(profile)
        return u
    }

    // request calls the hub; done(data) gets the parsed JSON on success,
    // finished() runs after every outcome.
    function request(method, path, body, done, finished) {
        if (token === "") {
            error = i18n("No token configured")
            return
        }
        const xhr = new XMLHttpRequest()
        xhr.open(method, url(path))
        xhr.setRequestHeader("Authorization", "Bearer " + token)
        if (body) xhr.setRequestHeader("Content-Type", "application/json")
        xhr.onreadystatechange = function () {
            if (xhr.readyState !== XMLHttpRequest.DONE) return
            if (finished) finished()
            let data = null
            try { data = JSON.parse(xhr.responseText) } catch (e) {}
            if (xhr.status === 200) {
                error = ""
                if (data && data.status && data.light) { hubState = data; lastSeen = Date.now() }
                if (done) done(data)
            } else {
                error = xhr.status === 0 ? i18n("Hub not reachable") : (data && data.error ? data.error : "HTTP " + xhr.status)
            }
        }
        xhr.send(body ? JSON.stringify(body) : null)
    }

    // One status poll at a time: a hanging request must not pile up more.
    property bool polling: false
    function refresh() {
        if (polling) return
        polling = true
        pollGuard.restart()
        request("GET", "/api/v1/status", null, null, function () { polling = false; pollGuard.stop() })
    }
    Timer {
        id: pollGuard
        interval: 30000
        onTriggered: root.polling = false
    }
    function setOverride(status, mode, duration) {
        const b = { status: status, mode: mode }
        if (mode === "duration") b.duration = duration
        request("PUT", "/api/v1/override", b, null)
    }
    function clearOverride() { request("DELETE", "/api/v1/override", null, null) }
    // workStatus: "dnd", "working" or "" for the hub's setting.
    function startPomodoro(work, brk, workStatus) {
        const b = { work: work, "break": brk, cycles: 4 }
        if (workStatus) b.work_status = workStatus
        request("PUT", "/api/v1/pomodoro", b, null)
    }
    function stopPomodoro() { request("DELETE", "/api/v1/pomodoro", null, null) }
    function setMessage(text) { request(text === "" ? "DELETE" : "PUT", "/api/v1/message", text === "" ? null : { text: text }, null) }

    // ---- live updates: read the SSE stream with a streaming XHR.
    //
    // Qt (6.12) doesn't cancel an HTTP/2 stream when an XHR is aborted: the
    // request stays open on the hub. A widget that aborts regularly fills the
    // connection up to the proxy's stream limit (Caddy: 250), after which
    // every request hangs. So the widget never aborts in normal operation:
    // the hub ends each stream after a few minutes and the widget reconnects.
    // Abort is left for config changes and a stream that went silent.
    property var stream: null
    property int streamPos: 0
    property double streamSeen: 0 // last sign of life on the stream (opened, data, keep-alive)
    property double lastSeen: 0   // last state received from the hub, by stream or poll

    function connectStream() {
        // Detach before aborting: abort() reports DONE synchronously, and the
        // handler must not take that for a dropped stream and reconnect.
        const old = stream
        stream = null
        live = false
        reconnect.stop()
        if (old) old.abort()
        if (token === "" || hub === "") return
        const xhr = new XMLHttpRequest()
        stream = xhr
        streamPos = 0
        streamSeen = Date.now()
        xhr.open("GET", url("/api/v1/events"))
        xhr.setRequestHeader("Authorization", "Bearer " + token)
        xhr.setRequestHeader("Accept", "text/event-stream")
        xhr.onreadystatechange = function () {
            if (xhr !== stream) return
            if (xhr.readyState === XMLHttpRequest.LOADING || xhr.readyState === XMLHttpRequest.DONE) {
                streamSeen = Date.now()
                const text = xhr.responseText
                let end
                while ((end = text.indexOf("\n\n", streamPos)) >= 0) {
                    const block = text.substring(streamPos, end)
                    streamPos = end + 2
                    const m = block.match(/^event: state\ndata: (.*)$/m)
                    if (m) {
                        try {
                            hubState = JSON.parse(block.substring(block.indexOf("data: ") + 6))
                            error = ""
                            live = true
                            lastSeen = Date.now()
                        } catch (e) {}
                    }
                }
            }
            if (xhr.readyState === XMLHttpRequest.DONE) {
                stream = null
                live = false
                reconnect.restart()
            }
        }
        xhr.send()
    }

    Timer {
        id: reconnect
        interval: 5000
        onTriggered: root.connectStream()
    }
    // Config changes arrive one property at a time; open one stream for all.
    Timer {
        id: restartStream
        interval: 200
        onTriggered: root.connectStream()
    }

    // Watchdog and fallback, every 15 s:
    // - the hub sends a keep-alive every 25 s; a stream silent for 70 s is
    //   dead, so open a new one;
    // - poll while the stream is down;
    // - never show an old state as current: after a minute without news
    //   the widget dims and says since when.
    Timer {
        interval: 15000
        running: true
        repeat: true
        triggeredOnStart: true
        onTriggered: {
            const now = Date.now()
            if (root.stream && now - root.streamSeen > 70000) root.connectStream()
            if (!root.live) root.refresh()
            if (root.lastSeen > 0 && now - root.lastSeen > 60000 && root.error === "")
                root.error = i18n("No update since %1", root.time(root.lastSeen))
        }
    }
    Timer {
        interval: 500
        running: root.hubState !== null && /blink|alternate/.test(root.hubState.light)
        repeat: true
        onTriggered: root.blinkOn = !root.blinkOn
        onRunningChanged: root.blinkOn = true
    }

    Component.onCompleted: restartStream.restart()
    onHubChanged: restartStream.restart()
    onTokenChanged: restartStream.restart()
    onProfileChanged: restartStream.restart()

    Plasmoid.icon: "user-busy"
    Plasmoid.status: hubState && hubState.status !== "away" ? PlasmaCore.Types.ActiveStatus : PlasmaCore.Types.PassiveStatus
    toolTipMainText: error !== "" ? i18n("doormutex: %1", error)
        : hubState ? (hubState.phase === "fault" ? i18n("Fault") : statusText[hubState.status]) : "doormutex"
    toolTipSubText: hubState ? describe(hubState) + (hubState.until ? " · " + i18n("until %1", time(hubState.until)) : "") : ""

    compactRepresentation: CompactRepresentation {}
    fullRepresentation: FullRepresentation {}

    Plasmoid.contextualActions: [
        PlasmaCore.Action {
            text: i18n("Red for 30 min")
            icon.name: "user-busy"
            onTriggered: root.setOverride("dnd", "duration", "30m")
        },
        PlasmaCore.Action {
            text: i18n("Green for 30 min")
            icon.name: "user-available"
            onTriggered: root.setOverride("free", "duration", "30m")
        },
        PlasmaCore.Action {
            text: i18n("Automatic")
            icon.name: "view-refresh"
            onTriggered: root.clearOverride()
        },
        PlasmaCore.Action {
            text: i18n("Open web UI")
            icon.name: "internet-web-browser"
            onTriggered: Qt.openUrlExternally(root.hub + "/")
        }
    ]
}
