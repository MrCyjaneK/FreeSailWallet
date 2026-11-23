TARGET = harbour-freesailwallet

CONFIG += sailfishapp c++11
QT += network

HEADERS += src/bridge.h \
    src/backend.h \
    src/passcardshape.h \
    src/transiticon.h
SOURCES += src/main.cpp \
    src/bridge.cpp \
    src/backend.cpp \
    src/passcardshape.cpp \
    src/transiticon.cpp

SAILFISHAPP_ICONS = 86x86 108x108 128x128 172x172 256x256

LIBS += -L$$PWD -lharbourfreesailwalletengine

libengine.target = $$PWD/libharbourfreesailwalletengine.so
libengine.commands = cd $$PWD/go && CGO_ENABLED=1 go build -mod=vendor -buildmode=c-shared -o $$PWD/libharbourfreesailwalletengine.so .
libengine.depends = go/export.go go/go.mod go/go.sum go/sail/sail.go \
    go/engine/wallet/store.go go/engine/pass/inspect.go go/engine/pass/barcode.go \
    go/sailpkg/embed.go go/sailpkg/rpm.go go/sailpkg/cpio.go go/sailpkg/patch.go go/sailpkg/tree.go
QMAKE_EXTRA_TARGETS += libengine
PRE_TARGETDEPS += $$PWD/libharbourfreesailwalletengine.so

sailqml.target = $$PWD/harbour-freesailwallet-sailqml
sailqml.commands = cd $$PWD/go && CGO_ENABLED=0 go build -mod=vendor -o $$PWD/harbour-freesailwallet-sailqml ./cmd/sailqml
sailqml.depends = go/cmd/sailqml/main.go go/sailpkg/embed.go go/sailpkg/rpm.go \
    go/sailpkg/cpio.go go/sailpkg/patch.go go/sailpkg/tree.go \
    go/sailpkg/harbour-sailwallet-1.1.0-1.aarch64.rpm \
    $$files(go/sailpkg/patches/*, true)
QMAKE_EXTRA_TARGETS += sailqml
PRE_TARGETDEPS += $$PWD/harbour-freesailwallet-sailqml

sailqmlbin.files = $$PWD/harbour-freesailwallet-sailqml
sailqmlbin.path = /usr/bin
INSTALLS += sailqmlbin

isEmpty(LIBDIR): LIBDIR = /usr/lib64
enginelib.files = $$PWD/libharbourfreesailwalletengine.so
enginelib.path = $$LIBDIR
INSTALLS += enginelib

translations.files = $$files(translations/*.qm)
translations.path = /usr/share/$$TARGET/translations
INSTALLS += translations

licenses.files = $$files(licenses/*.txt)
licenses.path = /usr/share/$$TARGET/licenses
INSTALLS += licenses

DISTFILES += $$files(qml/*, true) \
    rpm/harbour-freesailwallet.spec \
    harbour-freesailwallet.desktop
