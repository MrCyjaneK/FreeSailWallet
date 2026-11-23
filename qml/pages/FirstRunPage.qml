import QtQuick 2.6
import Sailfish.Silica 1.0
Page {
    id: page
    allowedOrientations: Orientation.Portrait
    backNavigation: false
    SilicaFlickable {
        anchors.fill: parent
        contentHeight: column.height + Theme.paddingLarge
        Column {
            id: column
            width: parent.width
            spacing: Theme.paddingMedium
            PageHeader {
                title: qsTr("Licenses")
            }
            Label {
                objectName: "firstRunIntro"
                color: Theme.highlightColor
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                text: qsTr("FreeSailWallet uses two licenses. Accept both to open the wallet.")
            }
            SectionHeader {
                text: qsTr("SailWallet")
            }
            Label {
                color: Theme.secondaryHighlightColor
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                font.pixelSize: Theme.fontSizeSmall
                text: qsTr("SailWallet is proprietary software by Jaroslav Macušík. FreeSailWallet redistributes the unmodified installation package.")
            }
            Label {
                objectName: "sailwalletLicenseText"
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                font.pixelSize: Theme.fontSizeExtraSmall
                color: Theme.secondaryColor
                textFormat: Text.PlainText
                text: appInfo.originalPackage.licenseText || ""
            }
            TextSwitch {
                id: sailSwitch
                objectName: "acceptSailWallet"
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                text: qsTr("I accept the SailWallet license")
            }
            SectionHeader {
                text: qsTr("Patcher")
            }
            Label {
                color: Theme.secondaryHighlightColor
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                font.pixelSize: Theme.fontSizeSmall
                text: qsTr("The program that extracts the SailWallet package and applies FreeSailWallet's patches is licensed under the GNU GPL.")
            }
            Label {
                objectName: "patcherLicenseText"
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                font.pixelSize: Theme.fontSizeExtraSmall
                color: Theme.secondaryColor
                textFormat: Text.PlainText
                text: appInfo.appLicenseText()
            }
            TextSwitch {
                id: patcherSwitch
                objectName: "acceptPatcher"
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                text: qsTr("I accept the Patcher license (GNU GPL)")
            }
            Button {
                objectName: "acceptLicenses"
                anchors.horizontalCenter: parent.horizontalCenter
                enabled: patcherSwitch.checked && sailSwitch.checked
                text: qsTr("Continue")
                onClicked: {
                    appSettings.acceptLicenses()
                    pageStack.replace(Qt.resolvedUrl("MainPage.qml"))
                }
            }
            Item { width: 1; height: Theme.paddingLarge }
        }
        VerticalScrollDecorator { }
    }
}
