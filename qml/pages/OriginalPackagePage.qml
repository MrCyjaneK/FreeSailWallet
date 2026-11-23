import QtQuick 2.6
import Sailfish.Silica 1.0
Page {
    id: page
    allowedOrientations: Orientation.Portrait
    readonly property var pkg: appInfo.originalPackage
    SilicaFlickable {
        anchors.fill: parent
        contentHeight: column.height + Theme.paddingLarge
        Column {
            id: column
            width: parent.width
            spacing: Theme.paddingMedium
            PageHeader {
                title: qsTr("Original package")
            }
            Label {
                objectName: "originalPackageNotice"
                color: Theme.highlightColor
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                text: qsTr("FreeSailWallet redistributes the SailWallet installation package unmodified. When FreeSailWallet is installed, the plaintext QML files are extracted from that package and FreeSailWallet's patches are applied on this device. SailWallet is Jaroslav Macušík's software. It is not FreeSailWallet, and its author does not endorse this app.")
            }
            Label {
                objectName: "originalPackageFile"
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                color: Theme.highlightColor
                font.pixelSize: Theme.fontSizeSmall
                text: pkg.fileName || ""
            }
            Label {
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                font.pixelSize: Theme.fontSizeSmall
                color: Theme.secondaryHighlightColor
                text: qsTr("Version %1-%2 (%3), %4 bytes").arg(pkg.version || "").arg(pkg.release || "").arg(pkg.arch || "").arg(pkg.size || "")
            }
            Label {
                objectName: "originalPackageSum"
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.WrapAnywhere
                font.pixelSize: Theme.fontSizeExtraSmall
                color: Theme.secondaryHighlightColor
                text: pkg.sha256 || ""
            }
            Label {
                objectName: "originalPackageHomepage"
                visible: (pkg.homepage || "").length > 0
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                color: homepageArea.pressed ? Theme.highlightColor : Theme.primaryColor
                text: pkg.homepage || ""
                MouseArea {
                    id: homepageArea
                    anchors.fill: parent
                    onClicked: Qt.openUrlExternally(pkg.homepage)
                }
            }
            SectionHeader {
                text: qsTr("SailWallet license")
            }
            Label {
                objectName: "originalPackageLicense"
                x: Theme.horizontalPageMargin
                width: parent.width - 2 * x
                wrapMode: Text.Wrap
                font.pixelSize: Theme.fontSizeExtraSmall
                color: Theme.secondaryHighlightColor
                textFormat: Text.PlainText
                text: pkg.licenseText || ""
            }
            Item { width: 1; height: Theme.paddingLarge }
        }
        VerticalScrollDecorator { }
    }
}
