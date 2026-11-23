#ifndef BACKEND_H
#define BACKEND_H

#include <QAbstractListModel>
#include <QObject>
#include <QQuickImageProvider>
#include <QStringList>
#include <QVariantList>

class PassListModel : public QAbstractListModel
{
    Q_OBJECT
    Q_PROPERTY(int count READ rowCount NOTIFY countChanged)
    Q_PROPERTY(bool loadFailed READ loadFailed NOTIFY loadFailedChanged)
    Q_PROPERTY(bool archiveView READ archiveView WRITE setArchiveView NOTIFY archiveViewChanged)
    Q_PROPERTY(QVariantMap front READ front NOTIFY frontChanged)

public:
    explicit PassListModel(QObject *parent = nullptr);
    int rowCount(const QModelIndex &parent = QModelIndex()) const override;
    QVariant data(const QModelIndex &index, int role) const override;
    QHash<int, QByteArray> roleNames() const override;
    bool loadFailed() const { return m_loadFailed; }
    bool archiveView() const { return m_archiveView; }
    QVariantMap front() const { return m_front; }
    void setArchiveView(bool on);
    Q_INVOKABLE QVariantMap get(const QVariant &passId) const;
    void reload();
    void moveToFront(const QString &passId);

signals:
    void countChanged();
    void loadFailedChanged();
    void archiveViewChanged();
    void frontChanged();

private:
    QList<QVariantMap> m_rows;
    QVariantMap m_front;
    QHash<int, QByteArray> m_roles;
    bool m_loadFailed = false;
    bool m_archiveView = false;
};

class Wallet : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool storageAvailable READ storageAvailable NOTIFY statusChanged)
    Q_PROPERTY(int passCount READ passCount NOTIFY statusChanged)
    Q_PROPERTY(int activeCount READ activeCount NOTIFY statusChanged)
    Q_PROPERTY(int archivedCount READ archivedCount NOTIFY statusChanged)
    Q_PROPERTY(bool newerDatabase READ newerDatabase NOTIFY statusChanged)
    Q_PROPERTY(QString newerDatabaseVersion READ newerDatabaseVersion NOTIFY statusChanged)

public:
    explicit Wallet(PassListModel *model, QObject *parent = nullptr);
    bool storageAvailable() const { return m_storage; }
    int passCount() const { return m_count; }
    int activeCount() const { return m_active; }
    int archivedCount() const { return m_archived; }
    bool newerDatabase() const { return m_newer; }
    QString newerDatabaseVersion() const { return m_newerVersion; }
    void refresh();
    Q_INVOKABLE bool removePass(const QVariant &passId);
    Q_INVOKABLE bool archive(const QVariant &passId);
    Q_INVOKABLE bool unarchive(const QVariant &passId);
    Q_INVOKABLE bool bringToFront(const QVariant &passId);
    Q_INVOKABLE void setAutoUpdate(const QVariant &passId, bool enabled);
    Q_INVOKABLE QString startEmptyWallet();
    Q_INVOKABLE QVariantMap ownCard(const QVariant &passId);
    Q_INVOKABLE QVariantMap shareResource(const QVariant &passId);

signals:
    void statusChanged();
    void autoUpdateChanged(const QVariant &passId, bool enabled);

private:
    void readStatus();
    PassListModel *m_model;
    bool m_storage = false;
    int m_count = 0;
    int m_active = 0;
    int m_archived = 0;
    bool m_newer = false;
    QString m_newerVersion;
};

class PassImporter : public QObject
{
    Q_OBJECT

public:
    explicit PassImporter(Wallet *wallet, QObject *parent = nullptr);
    Q_INVOKABLE QVariantMap inspectFile(const QString &path);
    Q_INVOKABLE QVariantList pendingUpdates(const QVariantList &paths);
    Q_INVOKABLE QVariantList importFiles(const QVariantList &paths, bool replaceExisting = false);
    Q_INVOKABLE QVariantMap addInspected(bool autoUpdate);
    Q_INVOKABLE QVariantMap replaceOwnCard(const QVariant &passId);
    Q_INVOKABLE QVariantMap inspectOwnCard(const QString &serial, const QString &name, const QString &code,
                                           const QString &format, const QString &color, const QVariantMap &fields);
    Q_INVOKABLE QVariantList ownCardColors();
    Q_INVOKABLE QVariantList ownCardIcons(const QString &templateId);
    Q_INVOKABLE QVariantList ownCardCurrencies();
    Q_INVOKABLE QString defaultOwnCardCurrency();
    Q_INVOKABLE QString formatOwnCardDate(const QString &isoDate);
    Q_INVOKABLE QString newOwnCardSerialNumber();

signals:
    void ownCardReplaced(const QVariant &passId);

private:
    static QStringList pathList(const QVariantList &paths);
    Wallet *m_wallet;
};

class PassUpdater : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool busy READ busy NOTIFY busyChanged)
    Q_PROPERTY(QStringList manualUpdates READ manualUpdates NOTIFY manualUpdatesChanged)

public:
    explicit PassUpdater(Wallet *wallet, QObject *parent = nullptr);
    bool busy() const { return m_busy; }
    QStringList manualUpdates() const { return m_manual; }
    Q_INVOKABLE void updatePass(const QVariant &passId);
    Q_INVOKABLE void updateAll();
    Q_INVOKABLE void cancel();

signals:
    void busyChanged();
    void manualUpdatesChanged();
    void updateFinished(const QVariant &passId, const QString &status, bool manual, const QString &reason);
    void autoUpdated(const QVariant &passId, const QStringList &messages);

private:
    void setBusy(bool busy);
    void deliver(const QVariant &result, bool manual);
    Wallet *m_wallet;
    bool m_busy = false;
    QStringList m_manual;
};

class PassDownloader : public QObject
{
    Q_OBJECT

public:
    explicit PassDownloader(QObject *parent = nullptr);
    Q_INVOKABLE void download(const QString &url);
    Q_INVOKABLE void findPassOnPage();
    Q_INVOKABLE void cancel();
    Q_INVOKABLE void removeDownloaded();
    Q_INVOKABLE QString hostName(const QString &address) const;

signals:
    void finished(const QString &status, const QString &filePath);

private:
    void start(const QString &method, const QVariantMap &args);
    bool m_busy = false;
};

class CodeScanner : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool busy READ busy NOTIFY busyChanged)

public:
    explicit CodeScanner(QObject *parent = nullptr);
    bool busy() const { return m_busy; }
    Q_INVOKABLE void scan(const QString &image, const QRectF &area);
    Q_INVOKABLE QVariantList codeTypes();
    Q_INVOKABLE bool isWebAddress(const QString &text);

signals:
    void busyChanged();
    void found(const QString &text, const QString &format, bool exact);

private:
    bool m_busy = false;
};

class WalletBackup : public QObject
{
    Q_OBJECT

public:
    explicit WalletBackup(Wallet *wallet, QObject *parent = nullptr);
    Q_INVOKABLE QVariantMap createBackup();
    Q_INVOKABLE QVariantMap inspectBackup(const QString &path);
    Q_INVOKABLE QVariantList restoreBackup(const QString &path);
    Q_INVOKABLE QVariantMap shareResource(const QString &filePath);

private:
    Wallet *m_wallet;
};

class AppSettings : public QObject
{
    Q_OBJECT
    Q_PROPERTY(bool internetAllowed READ internetAllowed WRITE setInternetAllowed NOTIFY internetAllowedChanged)
    Q_PROPERTY(bool licensesAccepted READ licensesAccepted NOTIFY licensesAcceptedChanged)

public:
    explicit AppSettings(PassUpdater *updater, QObject *parent = nullptr);
    bool internetAllowed() const { return m_allowed; }
    void setInternetAllowed(bool allowed);
    bool licensesAccepted() const { return m_licensesAccepted; }
    Q_INVOKABLE void acceptLicenses();
    Q_INVOKABLE void resetLicenseAcceptance();

signals:
    void internetAllowedChanged();
    void licensesAcceptedChanged();

private:
    PassUpdater *m_updater;
    bool m_allowed = false;
    bool m_licensesAccepted = false;
};

class AppInfo : public QObject
{
    Q_OBJECT
    Q_PROPERTY(QString version READ version CONSTANT)
    Q_PROPERTY(QString iconPath READ iconPath CONSTANT)
    Q_PROPERTY(QString authorName READ authorName CONSTANT)
    Q_PROPERTY(QString authorContact READ authorContact CONSTANT)
    Q_PROPERTY(QString authorWebsite READ authorWebsite CONSTANT)
    Q_PROPERTY(QVariantList licenses READ licenses CONSTANT)
    Q_PROPERTY(QVariantMap originalPackage READ originalPackage CONSTANT)

public:
    explicit AppInfo(QObject *parent = nullptr);
    QString version() const { return QStringLiteral("0.1"); }
    QString iconPath() const;
    QString authorName() const { return QStringLiteral("Czarek Nakamoto"); }
    QString authorContact() const { return QString(); }
    QString authorWebsite() const { return QStringLiteral("https://github.com/mrcyjanek/harbour-freesailwallet"); }
    QVariantList licenses() const { return m_licenses; }
    QVariantMap originalPackage() const;
    Q_INVOKABLE QString licenseText(const QString &id) const;
    Q_INVOKABLE QString appLicenseText() const;

private:
    QString readLicense(const QString &fileName) const;
    QVariantList m_licenses;
};

class OpticalCenter : public QObject
{
    Q_OBJECT

public:
    explicit OpticalCenter(QObject *parent = nullptr) : QObject(parent) {}
    Q_INVOKABLE double offset(const QString &, const QVariant &) const { return 0; }
};

class FileOpener : public QObject
{
    Q_OBJECT
    Q_PROPERTY(QStringList initialFiles READ initialFiles CONSTANT)

public:
    explicit FileOpener(QObject *parent = nullptr);
    QStringList initialFiles() const { return m_initial; }
    void takeArguments(const QStringList &arguments);

signals:
    void openRequested(const QVariantList &files);

private:
    QStringList m_initial;
};

class PassImageProvider : public QQuickImageProvider
{
public:
    PassImageProvider();
    QImage requestImage(const QString &id, QSize *size, const QSize &requestedSize) override;
};

#endif
