Name: harbour-freesailwallet

Summary: Tickets, coupons, codes and loyalty cards
Version: 0.1
Release: 2
Group: Qt/Qt
License: BSD
URL: https://github.com/mrcyjanek/harbour-freesailwallet
Source0: %{name}-%{version}.tar.bz2
Requires: sailfishsilica-qt5 >= 0.10.9
BuildRequires: golang
BuildRequires: pkgconfig(sailfishapp) >= 1.0.2
BuildRequires: pkgconfig(Qt5Core)
BuildRequires: pkgconfig(Qt5Gui)
BuildRequires: pkgconfig(Qt5Qml)
BuildRequires: pkgconfig(Qt5Quick)
BuildRequires: pkgconfig(Qt5Network)
BuildRequires: desktop-file-utils

%description
Keep tickets, coupons, codes and loyalty cards, and read Apple Wallet
.pkpass files. Cards stay on the device. Internet access is off until
it is turned on in Settings.

%prep
%autosetup -n %{name}-%{version}

%build
%qmake5
%make_build

%install
%qmake5_install
mkdir -p %{buildroot}%{_libdir}
install -m 755 libharbourfreesailwalletengine.so %{buildroot}%{_libdir}/
install -m 755 harbour-freesailwallet-sailqml %{buildroot}%{_bindir}/
%{buildroot}%{_bindir}/harbour-freesailwallet-sailqml prune --dest %{buildroot}%{_datadir}/%{name}/qml
desktop-file-install --delete-original \
    --dir %{buildroot}%{_datadir}/applications \
    %{buildroot}%{_datadir}/applications/*.desktop

%post
%{_bindir}/harbour-freesailwallet-sailqml install --dest %{_datadir}/%{name}/qml

%postun
if [ "$1" -eq 0 ]; then
    manifest="%{_datadir}/%{name}/qml/.sailpkg-files"
    if [ -f "$manifest" ]; then
        while IFS= read -r rel || [ -n "$rel" ]; do
            if [ -n "$rel" ]; then
                rm -f "%{_datadir}/%{name}/qml/$rel"
            fi
        done < "$manifest"
        rm -f "$manifest"
    fi
fi

%files
%{_bindir}/%{name}
%{_bindir}/harbour-freesailwallet-sailqml
%{_libdir}/libharbourfreesailwalletengine.so
%{_datadir}/%{name}
%{_datadir}/applications/%{name}.desktop
%{_datadir}/icons/hicolor/*/apps/%{name}.png
