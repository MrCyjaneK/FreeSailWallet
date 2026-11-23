#include "backend.h"
#include "passcardshape.h"
#include "transiticon.h"

#include "bridge.h"

#include <QCoreApplication>
#include <QDir>
#include <QGuiApplication>
#include <QLocale>
#include <QQmlContext>
#include <QQuickView>
#include <QStandardPaths>
#include <QTranslator>
#include <sailfishapp.h>

static QString currencyFor(const QLocale &locale)
{
    const QString name = locale.name();
    if (name.startsWith(QLatin1String("en_US")))
        return QStringLiteral("USD");
    if (name.startsWith(QLatin1String("en_GB")))
        return QStringLiteral("GBP");
    if (name.startsWith(QLatin1String("cs")))
        return QStringLiteral("CZK");
    if (name.startsWith(QLatin1String("pl")))
        return QStringLiteral("PLN");
    if (name.startsWith(QLatin1String("sv")))
        return QStringLiteral("SEK");
    if (name.startsWith(QLatin1String("da")))
        return QStringLiteral("DKK");
    if (name.startsWith(QLatin1String("nb")) || name.startsWith(QLatin1String("nn"))
        || name.startsWith(QLatin1String("no")))
        return QStringLiteral("NOK");
    if (name.startsWith(QLatin1String("ja")))
        return QStringLiteral("JPY");
    if (name.contains(QLatin1String("_CH")))
        return QStringLiteral("CHF");
    return QStringLiteral("EUR");
}

static QString walletDirectory()
{
    const QString dir = QDir::homePath() + QStringLiteral("/.local/share/net.mrcyjanek/FreeSailWallet");
    QDir().mkpath(dir);
    return dir;
}

int main(int argc, char *argv[])
{
    QCoreApplication::setOrganizationName(QStringLiteral("net.mrcyjanek"));
    QCoreApplication::setOrganizationDomain(QStringLiteral("mrcyjanek.net"));
    QCoreApplication::setApplicationName(QStringLiteral("FreeSailWallet"));

    QScopedPointer<QGuiApplication> app(SailfishApp::application(argc, argv));
    app->setQuitOnLastWindowClosed(true);
    app->setOrganizationName(QStringLiteral("net.mrcyjanek"));
    app->setOrganizationDomain(QStringLiteral("mrcyjanek.net"));
    app->setApplicationName(QStringLiteral("FreeSailWallet"));
    app->setApplicationVersion(QStringLiteral("0.1"));

    QTranslator translator;
    const QString translationDir = SailfishApp::pathTo(QStringLiteral("translations")).toLocalFile();
    if (translator.load(QLocale(), QStringLiteral("harbour-freesailwallet"), QStringLiteral("-"),
                        translationDir, QStringLiteral(".qm")))
        app->installTranslator(&translator);

    const QString dataDir = walletDirectory();
    const QString downloads = QStandardPaths::writableLocation(QStandardPaths::DownloadLocation);
    Bridge::call(QStringLiteral("open"),
                 {{QStringLiteral("dataDir"), dataDir},
                  {QStringLiteral("downloadsDir"), downloads},
                  {QStringLiteral("locale"), QLocale().name()},
                  {QStringLiteral("scale"), 2},
                  {QStringLiteral("currency"), currencyFor(QLocale())}});

    QScopedPointer<QQuickView> view(SailfishApp::createView());
    qmlRegisterType<PassCardShape>("harbour.freesailwallet", 1, 0, "PassCardShape");
    qmlRegisterType<TransitIcon>("harbour.freesailwallet", 1, 0, "TransitIcon");
    view->engine()->addImageProvider(QStringLiteral("pass"), new PassImageProvider);

    PassListModel passModel;
    Wallet wallet(&passModel);
    PassImporter passImporter(&wallet);
    PassUpdater passUpdater(&wallet);
    PassDownloader passDownloader;
    CodeScanner codeScanner;
    WalletBackup walletBackup(&wallet);
    AppSettings appSettings(&passUpdater);
    AppInfo appInfo;
    FileOpener fileOpener;
    fileOpener.takeArguments(app->arguments());
    OpticalCenter opticalCenter;

    QQmlContext *context = view->rootContext();
    context->setContextProperty(QStringLiteral("wallet"), &wallet);
    context->setContextProperty(QStringLiteral("passModel"), &passModel);
    context->setContextProperty(QStringLiteral("passImporter"), &passImporter);
    context->setContextProperty(QStringLiteral("passUpdater"), &passUpdater);
    context->setContextProperty(QStringLiteral("passDownloader"), &passDownloader);
    context->setContextProperty(QStringLiteral("codeScanner"), &codeScanner);
    context->setContextProperty(QStringLiteral("walletBackup"), &walletBackup);
    context->setContextProperty(QStringLiteral("appSettings"), &appSettings);
    context->setContextProperty(QStringLiteral("appInfo"), &appInfo);
    context->setContextProperty(QStringLiteral("fileOpener"), &fileOpener);
    context->setContextProperty(QStringLiteral("opticalCenter"), &opticalCenter);

    if (appSettings.internetAllowed())
        passUpdater.updateAll();

    view->setSource(SailfishApp::pathToMainQml());
    view->show();
    return app->exec();
}
