import QtQuick 2.6
import Sailfish.Silica 1.0
import "pages"
import "cover"
ApplicationWindow {
    id: app
    initialPage: appSettings.licensesAccepted ? mainPage : firstRunPage
    cover: Component { CoverPage { } }
    allowedOrientations: Orientation.All
    Component {
        id: mainPage
        MainPage { }
    }
    Component {
        id: firstRunPage
        FirstRunPage { }
    }
}
