import QtQuick
import QtQuick.Controls as QQC2
import QtQuick.Layouts
import org.kde.kirigami as Kirigami
import org.kde.kcmutils as KCM

KCM.SimpleKCM {
    property alias cfg_hubUrl: hubUrl.text
    property alias cfg_token: token.text
    property alias cfg_profile: profile.text

    Kirigami.FormLayout {
        QQC2.TextField {
            id: hubUrl
            Kirigami.FormData.label: i18n("Hub:")
            placeholderText: "http://hub.lan:8080"
        }
        Kirigami.PasswordField {
            id: token
            Kirigami.FormData.label: i18n("Token:")
            placeholderText: "dm_…"
        }
        QQC2.TextField {
            id: profile
            Kirigami.FormData.label: i18n("Profile:")
            placeholderText: i18n("the token's profile")
        }
        QQC2.Label {
            text: i18n("Create a token on the hub: doormutex-hub token create widget office")
            wrapMode: Text.Wrap
            opacity: 0.7
            Layout.fillWidth: true
        }
    }
}
