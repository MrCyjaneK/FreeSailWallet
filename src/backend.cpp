#include "backend.h"

#include "bridge.h"

#include <QBuffer>
#include <QCoreApplication>
#include <QDir>
#include <QFile>
#include <QImage>
#include <unistd.h>
#include <QJsonDocument>
#include <QPainter>
#include <QSettings>
#include <QStandardPaths>
#include <QThread>
#include <QUrl>
#include <QVariantMap>

namespace {

QString localPath(const QString &value);

QString settingsFilePath()
{
    const QString dir = QStandardPaths::writableLocation(QStandardPaths::AppConfigLocation);
    QDir().mkpath(dir);
    return dir + QStringLiteral("/settings.conf");
}

class CallThread : public QThread
{
public:
    QString method;
    QVariantMap args;
    QVariant result;
    void run() override { result = Bridge::call(method, args); }
};

class ScanThread : public QThread
{
public:
    QString image;
    QRectF area;
    QVariant result;
    void run() override
    {
        QImage frame(localPath(image));
        if (!frame.isNull() && area.width() > 0 && area.height() > 0) {
            const QRect crop(int(area.x() * frame.width()), int(area.y() * frame.height()),
                             int(area.width() * frame.width()), int(area.height() * frame.height()));
            const QRect clipped = crop.intersected(frame.rect());
            if (clipped.isValid())
                frame = frame.copy(clipped);
        }
        QByteArray png;
        if (!frame.isNull()) {
            QBuffer buffer(&png);
            buffer.open(QIODevice::WriteOnly);
            frame.save(&buffer, "PNG");
        }
        result = Bridge::call(QStringLiteral("scan"),
                              {{QStringLiteral("png"), QString::fromLatin1(png.toBase64())}});
    }
};

QVariantMap withFileUrl(QVariant value)
{
    QVariantMap map = value.toMap();
    const QString path = map.value(QStringLiteral("path")).toString();
    if (!path.isEmpty())
        map.insert(QStringLiteral("data"), QUrl::fromLocalFile(path).toString());
    return map;
}

QString localPath(const QString &value)
{
    const QUrl url(value);
    if (url.isLocalFile())
        return url.toLocalFile();
    return value;
}

const char *kRoleNames[] = {
    "passId", "imageKey", "passStyle", "transitType", "logoText", "passLogoText", "ownCard",
    "logoSize", "initials", "hasLogo", "hasThumbnail", "hasStrip", "hasBackground", "hasFooter",
    "barcodeFormat", "barcodeShape", "barcodeAltText", "voided", "expirationTime",
    "backgroundColor", "foregroundColor", "labelColor", "headerFields", "primaryFields",
    "secondaryFields", "auxiliaryFields", "backFields", "passDescription", "organizationName",
    "canUpdate", "canShare", "autoUpdate", "archived"
};

} // namespace

PassListModel::PassListModel(QObject *parent)
    : QAbstractListModel(parent)
{
    for (int i = 0; i < int(sizeof(kRoleNames) / sizeof(kRoleNames[0])); ++i)
        m_roles.insert(Qt::UserRole + i, kRoleNames[i]);
}

int PassListModel::rowCount(const QModelIndex &parent) const
{
    if (parent.isValid())
        return 0;
    return m_rows.size();
}

QVariant PassListModel::data(const QModelIndex &index, int role) const
{
    if (!index.isValid() || index.row() < 0 || index.row() >= m_rows.size())
        return QVariant();
    return m_rows.at(index.row()).value(QString::fromUtf8(m_roles.value(role)));
}

QHash<int, QByteArray> PassListModel::roleNames() const
{
    return m_roles;
}

QVariantMap PassListModel::get(const QVariant &passId) const
{
    const QString id = passId.toString();
    for (const QVariantMap &row : m_rows) {
        if (row.value(QStringLiteral("passId")).toString() == id)
            return row;
    }
    const QVariantMap fresh = Bridge::call(QStringLiteral("get"),
                                            {{QStringLiteral("passId"), id}}).toMap();
    if (fresh.value(QStringLiteral("passId")).toString().isEmpty())
        return QVariantMap();
    return fresh;
}

void PassListModel::setArchiveView(bool on)
{
    if (m_archiveView == on)
        return;
    m_archiveView = on;
    emit archiveViewChanged();
    reload();
}

void PassListModel::reload()
{
    const QVariant status = Bridge::call(QStringLiteral("status"));
    const bool failed = status.toMap().value(QStringLiteral("loadFailed")).toBool();
    const QVariant listed = Bridge::call(QStringLiteral("list"),
                                          {{QStringLiteral("archived"), m_archiveView}});
    QList<QVariantMap> rows;
    const QVariantList items = listed.toList();
    for (const QVariant &item : items)
        rows.append(item.toMap());
    // Archive only swaps the list. The front card stays the current active pass.
    if (!m_archiveView)
        m_front = rows.isEmpty() ? QVariantMap() : rows.last();
    beginResetModel();
    m_rows = rows;
    endResetModel();
    emit frontChanged();
    if (m_loadFailed != failed) {
        m_loadFailed = failed;
        emit loadFailedChanged();
    }
    emit countChanged();
}

void PassListModel::moveToFront(const QString &passId)
{
    if (m_archiveView || passId.isEmpty())
        return;
    int from = -1;
    for (int i = 0; i < m_rows.size(); ++i) {
        if (m_rows.at(i).value(QStringLiteral("passId")).toString() == passId) {
            from = i;
            break;
        }
    }
    // The painted list is reversed, so the last row is the front card.
    if (from < 0 || from == m_rows.size() - 1)
        return;
    const QVariantMap row = m_rows.at(from);
    beginResetModel();
    m_rows.removeAt(from);
    m_rows.append(row);
    m_front = row;
    endResetModel();
    emit frontChanged();
}

Wallet::Wallet(PassListModel *model, QObject *parent)
    : QObject(parent)
    , m_model(model)
{
    refresh();
}

void Wallet::refresh()
{
    if (m_model)
        m_model->reload();
    readStatus();
}

void Wallet::readStatus()
{
    const QVariantMap status = Bridge::call(QStringLiteral("status")).toMap();
    const bool storage = status.value(QStringLiteral("storageAvailable")).toBool();
    const int count = status.value(QStringLiteral("passCount")).toInt();
    const int active = status.value(QStringLiteral("activeCount")).toInt();
    const int archived = status.value(QStringLiteral("archivedCount")).toInt();
    const bool newer = status.value(QStringLiteral("newerDatabase")).toBool();
    const QString version = status.value(QStringLiteral("newerDatabaseVersion")).toString();
    if (storage == m_storage && count == m_count && active == m_active && archived == m_archived
        && newer == m_newer && version == m_newerVersion)
        return;
    m_storage = storage;
    m_count = count;
    m_active = active;
    m_archived = archived;
    m_newer = newer;
    m_newerVersion = version;
    emit statusChanged();
}

bool Wallet::removePass(const QVariant &passId)
{
    const QVariantMap result = Bridge::call(QStringLiteral("removePass"),
                                            {{QStringLiteral("passId"), passId.toString()}}).toMap();
    const bool ok = result.value(QStringLiteral("ok")).toBool();
    if (ok)
        refresh();
    return ok;
}

bool Wallet::archive(const QVariant &passId)
{
    const QVariantMap result = Bridge::call(QStringLiteral("archive"),
                                            {{QStringLiteral("passId"), passId.toString()}}).toMap();
    const bool ok = result.value(QStringLiteral("ok")).toBool();
    if (ok)
        refresh();
    return ok;
}

bool Wallet::unarchive(const QVariant &passId)
{
    const QVariantMap result = Bridge::call(QStringLiteral("unarchive"),
                                            {{QStringLiteral("passId"), passId.toString()}}).toMap();
    const bool ok = result.value(QStringLiteral("ok")).toBool();
    if (ok)
        refresh();
    return ok;
}

bool Wallet::bringToFront(const QVariant &passId)
{
    const QVariantMap result = Bridge::call(QStringLiteral("bringToFront"),
                                            {{QStringLiteral("passId"), passId.toString()}}).toMap();
    if (!result.value(QStringLiteral("ok")).toBool())
        return false;
    if (result.value(QStringLiteral("moved")).toBool() && m_model)
        m_model->moveToFront(passId.toString());
    return true;
}

void Wallet::setAutoUpdate(const QVariant &passId, bool enabled)
{
    Bridge::call(QStringLiteral("setAutoUpdate"),
                 {{QStringLiteral("passId"), passId.toString()}, {QStringLiteral("enabled"), enabled}});
    refresh();
    emit autoUpdateChanged(passId.toString(), enabled);
}

QString Wallet::startEmptyWallet()
{
    const QString file = Bridge::call(QStringLiteral("startEmptyWallet")).toMap().value(QStringLiteral("file")).toString();
    refresh();
    return file;
}

QVariantMap Wallet::ownCard(const QVariant &passId)
{
    return Bridge::call(QStringLiteral("ownCard"), {{QStringLiteral("passId"), passId.toString()}}).toMap();
}

QVariantMap Wallet::shareResource(const QVariant &passId)
{
    return withFileUrl(Bridge::call(QStringLiteral("shareResource"),
                                    {{QStringLiteral("passId"), passId.toString()}}));
}

PassImporter::PassImporter(Wallet *wallet, QObject *parent)
    : QObject(parent)
    , m_wallet(wallet)
{
}

QStringList PassImporter::pathList(const QVariantList &paths)
{
    QStringList out;
    for (const QVariant &path : paths)
        out.append(localPath(path.toString()));
    return out;
}

QVariantMap PassImporter::inspectFile(const QString &path)
{
    return Bridge::call(QStringLiteral("inspectFile"), {{QStringLiteral("path"), localPath(path)}}).toMap();
}

QVariantList PassImporter::pendingUpdates(const QVariantList &paths)
{
    return Bridge::call(QStringLiteral("pendingUpdates"),
                        {{QStringLiteral("paths"), pathList(paths)}}).toList();
}

QVariantList PassImporter::importFiles(const QVariantList &paths, bool replaceExisting)
{
    const QVariantList results = Bridge::call(QStringLiteral("importFiles"),
                                              {{QStringLiteral("paths"), pathList(paths)},
                                               {QStringLiteral("replaceExisting"), replaceExisting}}).toList();
    if (m_wallet)
        m_wallet->refresh();
    return results;
}

QVariantMap PassImporter::addInspected(bool autoUpdate)
{
    const QVariantMap result = Bridge::call(QStringLiteral("addInspected"),
                                            {{QStringLiteral("autoUpdate"), autoUpdate}}).toMap();
    if (m_wallet)
        m_wallet->refresh();
    return result;
}

QVariantMap PassImporter::replaceOwnCard(const QVariant &passId)
{
    const QVariantMap result = Bridge::call(QStringLiteral("replaceOwnCard"),
                                            {{QStringLiteral("passId"), passId.toString()}}).toMap();
    if (result.value(QStringLiteral("status")).toString() != QLatin1String("failed") && m_wallet)
        m_wallet->refresh();
    emit ownCardReplaced(result.value(QStringLiteral("passId"), passId));
    return result;
}

QVariantMap PassImporter::inspectOwnCard(const QString &serial, const QString &name, const QString &code,
                                         const QString &format, const QString &color, const QVariantMap &fields)
{
    return Bridge::call(QStringLiteral("inspectOwnCard"),
                        {{QStringLiteral("serialNumber"), serial},
                         {QStringLiteral("name"), name},
                         {QStringLiteral("code"), code},
                         {QStringLiteral("format"), format},
                         {QStringLiteral("color"), color},
                         {QStringLiteral("fields"), fields}}).toMap();
}

QVariantList PassImporter::ownCardColors()
{
    return Bridge::call(QStringLiteral("ownCardColors")).toList();
}

QVariantList PassImporter::ownCardIcons(const QString &templateId)
{
    return Bridge::call(QStringLiteral("ownCardIcons"), {{QStringLiteral("templateId"), templateId}}).toList();
}

QVariantList PassImporter::ownCardCurrencies()
{
    return Bridge::call(QStringLiteral("ownCardCurrencies")).toList();
}

QString PassImporter::defaultOwnCardCurrency()
{
    return Bridge::call(QStringLiteral("defaultOwnCardCurrency")).toString();
}

QString PassImporter::formatOwnCardDate(const QString &isoDate)
{
    return Bridge::call(QStringLiteral("formatOwnCardDate"), {{QStringLiteral("isoDate"), isoDate}}).toString();
}

QString PassImporter::newOwnCardSerialNumber()
{
    return Bridge::call(QStringLiteral("newOwnCardSerialNumber")).toString();
}

PassUpdater::PassUpdater(Wallet *wallet, QObject *parent)
    : QObject(parent)
    , m_wallet(wallet)
{
}

void PassUpdater::setBusy(bool busy)
{
    if (m_busy == busy)
        return;
    m_busy = busy;
    emit busyChanged();
}

void PassUpdater::updatePass(const QVariant &passId)
{
    const QString id = passId.toString();
    if (!m_manual.contains(id)) {
        m_manual.append(id);
        emit manualUpdatesChanged();
    }
    setBusy(true);
    auto *job = new CallThread;
    job->method = QStringLiteral("updatePass");
    job->args = {{QStringLiteral("passId"), id}};
    connect(job, &QThread::finished, this, [this, job, id]() {
        deliver(job->result, true);
        m_manual.removeAll(id);
        emit manualUpdatesChanged();
        setBusy(m_manual.size() > 0);
        job->deleteLater();
    });
    job->start();
}

void PassUpdater::updateAll()
{
    setBusy(true);
    auto *job = new CallThread;
    job->method = QStringLiteral("updateAll");
    connect(job, &QThread::finished, this, [this, job]() {
        const QVariantList results = job->result.toMap().value(QStringLiteral("results")).toList();
        for (const QVariant &item : results)
            deliver(item, false);
        setBusy(false);
        job->deleteLater();
    });
    job->start();
}

void PassUpdater::cancel()
{
    Bridge::call(QStringLiteral("cancel"));
}

void PassUpdater::deliver(const QVariant &result, bool manual)
{
    const QVariantMap map = result.toMap();
    const QString status = map.value(QStringLiteral("status")).toString();
    const QVariant passId = map.value(QStringLiteral("passId"));
    QStringList messages;
    const QVariantList raw = map.value(QStringLiteral("messages")).toList();
    for (const QVariant &message : raw)
        messages.append(message.toString());
    if (status == QLatin1String("updated") && m_wallet)
        m_wallet->refresh();
    if (!manual)
        emit autoUpdated(passId, messages);
    emit updateFinished(passId, status, manual, map.value(QStringLiteral("reason")).toString());
}

PassDownloader::PassDownloader(QObject *parent)
    : QObject(parent)
{
}

void PassDownloader::download(const QString &url)
{
    start(QStringLiteral("download"), {{QStringLiteral("url"), url}});
}

void PassDownloader::findPassOnPage()
{
    start(QStringLiteral("findPassOnPage"), {});
}

void PassDownloader::start(const QString &method, const QVariantMap &args)
{
    m_busy = true;
    auto *job = new CallThread;
    job->method = method;
    job->args = args;
    connect(job, &QThread::finished, this, [this, job]() {
        m_busy = false;
        const QVariantMap map = job->result.toMap();
        emit finished(map.value(QStringLiteral("status")).toString(),
                      map.value(QStringLiteral("filePath")).toString());
        job->deleteLater();
    });
    job->start();
}

void PassDownloader::cancel()
{
    Bridge::call(QStringLiteral("cancel"));
}

void PassDownloader::removeDownloaded()
{
    Bridge::call(QStringLiteral("removeDownloaded"));
}

QString PassDownloader::hostName(const QString &address) const
{
    return QUrl(address).host();
}

CodeScanner::CodeScanner(QObject *parent)
    : QObject(parent)
{
}

void CodeScanner::scan(const QString &image, const QRectF &area)
{
    if (m_busy)
        return;
    m_busy = true;
    emit busyChanged();
    auto *job = new ScanThread;
    job->image = image;
    job->area = area;
    connect(job, &QThread::finished, this, [this, job]() {
        m_busy = false;
        emit busyChanged();
        const QVariantMap map = job->result.toMap();
        if (map.value(QStringLiteral("found")).toBool())
            emit found(map.value(QStringLiteral("text")).toString(),
                       map.value(QStringLiteral("format")).toString(),
                       map.value(QStringLiteral("exact")).toBool());
        job->deleteLater();
    });
    job->start();
}

QVariantList CodeScanner::codeTypes()
{
    return Bridge::call(QStringLiteral("codeTypes")).toList();
}

bool CodeScanner::isWebAddress(const QString &text)
{
    return Bridge::call(QStringLiteral("isWebAddress"), {{QStringLiteral("text"), text}}).toBool();
}

WalletBackup::WalletBackup(Wallet *wallet, QObject *parent)
    : QObject(parent)
    , m_wallet(wallet)
{
}

QVariantMap WalletBackup::createBackup()
{
    return Bridge::call(QStringLiteral("createBackup")).toMap();
}

QVariantMap WalletBackup::inspectBackup(const QString &path)
{
    return Bridge::call(QStringLiteral("inspectBackup"), {{QStringLiteral("path"), localPath(path)}}).toMap();
}

QVariantList WalletBackup::restoreBackup(const QString &path)
{
    const QVariantList results = Bridge::call(QStringLiteral("restoreBackup"),
                                              {{QStringLiteral("path"), localPath(path)}}).toList();
    if (m_wallet)
        m_wallet->refresh();
    return results;
}

QVariantMap WalletBackup::shareResource(const QString &filePath)
{
    return withFileUrl(Bridge::call(QStringLiteral("shareBackup"),
                                    {{QStringLiteral("filePath"), localPath(filePath)}}));
}

AppSettings::AppSettings(PassUpdater *updater, QObject *parent)
    : QObject(parent)
    , m_updater(updater)
{
    const QString key = QStringLiteral("network/internetAllowed");
    QSettings settings(settingsFilePath(), QSettings::IniFormat);
    if (settings.contains(key))
        m_allowed = settings.value(key).toBool();
    else {
        QSettings legacy;
        m_allowed = legacy.value(key, false).toBool();
        if (legacy.contains(key)) {
            settings.setValue(key, m_allowed);
            settings.sync();
        }
    }
    Bridge::call(QStringLiteral("setInternetAllowed"), {{QStringLiteral("value"), m_allowed}});
    m_licensesAccepted = settings.value(QStringLiteral("licenses/accepted"), false).toBool();
}

void AppSettings::setInternetAllowed(bool allowed)
{
    if (m_allowed == allowed)
        return;
    m_allowed = allowed;
    QSettings settings(settingsFilePath(), QSettings::IniFormat);
    settings.setValue(QStringLiteral("network/internetAllowed"), allowed);
    settings.sync();
    Bridge::call(QStringLiteral("setInternetAllowed"), {{QStringLiteral("value"), allowed}});
    emit internetAllowedChanged();
    if (allowed && m_updater)
        m_updater->updateAll();
}

void AppSettings::acceptLicenses()
{
    if (m_licensesAccepted)
        return;
    m_licensesAccepted = true;
    QSettings settings(settingsFilePath(), QSettings::IniFormat);
    settings.setValue(QStringLiteral("licenses/accepted"), true);
    settings.sync();
    emit licensesAcceptedChanged();
}

void AppSettings::resetLicenseAcceptance()
{
    if (!m_licensesAccepted)
        return;
    m_licensesAccepted = false;
    QSettings settings(settingsFilePath(), QSettings::IniFormat);
    settings.setValue(QStringLiteral("licenses/accepted"), false);
    settings.sync();
    emit licensesAcceptedChanged();
}

AppInfo::AppInfo(QObject *parent)
    : QObject(parent)
{
    const struct {
        const char *id;
        const char *name;
        const char *license;
    } libs[] = {
        {"zint", "Zint", "BSD"},
        {"zxing-cpp", "zxing-cpp", "Apache-2.0"},
        {"libzueci", "libzueci", "MIT"},
    };
    for (const auto &lib : libs) {
        m_licenses.append(QVariantMap{{QStringLiteral("id"), QString::fromLatin1(lib.id)},
                                      {QStringLiteral("name"), QString::fromLatin1(lib.name)},
                                      {QStringLiteral("license"), QString::fromLatin1(lib.license)}});
    }
}

QString AppInfo::iconPath() const
{
    return QStringLiteral("/usr/share/icons/hicolor/172x172/apps/harbour-freesailwallet.png");
}

QString AppInfo::licenseText(const QString &id) const
{
    return readLicense(id + QStringLiteral(".txt"));
}

QVariantMap AppInfo::originalPackage() const
{
    return Bridge::call(QStringLiteral("originalPackage")).toMap();
}

QString AppInfo::appLicenseText() const
{
    return readLicense(QStringLiteral("patcher.txt"));
}

QString AppInfo::readLicense(const QString &fileName) const
{
    const QString dir = QStandardPaths::locate(QStandardPaths::GenericDataLocation,
                                               QStringLiteral("harbour-freesailwallet/licenses/") + fileName);
    QFile file(dir);
    if (!file.open(QIODevice::ReadOnly))
        return QString();
    return QString::fromUtf8(file.readAll());
}

FileOpener::FileOpener(QObject *parent)
    : QObject(parent)
{
    // The Go runtime keeps threads alive after Qt returns, so the process
    // would stay running with no window.
    QObject::connect(qApp, &QCoreApplication::aboutToQuit, qApp, []() { _exit(0); });
}

void FileOpener::takeArguments(const QStringList &arguments)
{
    for (int i = 1; i < arguments.size(); ++i) {
        const QString path = localPath(arguments.at(i));
        if (path.isEmpty() || path.startsWith(QLatin1Char('-')))
            continue;
        m_initial.append(path);
    }
}

PassImageProvider::PassImageProvider()
    : QQuickImageProvider(QQuickImageProvider::Image)
{
}

QImage PassImageProvider::requestImage(const QString &id, QSize *size, const QSize &requestedSize)
{
    const int slash = id.indexOf(QLatin1Char('/'));
    const QString key = slash < 0 ? id : id.left(slash);
    const QString name = slash < 0 ? QString() : id.mid(slash + 1);
    int w = requestedSize.width();
    int h = requestedSize.height();
    if (w < 1)
        w = 256;
    if (h < 1)
        h = 256;
    const QByteArray png = Bridge::image(key, name, w, h);
    QImage image;
    image.loadFromData(png, "PNG");
    if (size)
        *size = image.size();
    return image;
}
