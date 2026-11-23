#include "bridge.h"

#include <QJsonArray>
#include <QJsonDocument>
#include <QJsonObject>
#include <QJsonValue>
#include <QSize>

extern "C" {
char *WalletCall(char *method, char *args);
void WalletFree(void *p);
void *WalletImage(char *key, char *name, int w, int h, int *outLen);
}

static QVariant fromJson(const QJsonValue &value, const QString &key)
{
    if (value.isObject()) {
        const QJsonObject object = value.toObject();
        if (key == QLatin1String("logoSize"))
            return QSize(object.value(QStringLiteral("width")).toInt(),
                         object.value(QStringLiteral("height")).toInt());
        QVariantMap map;
        for (auto it = object.begin(); it != object.end(); ++it)
            map.insert(it.key(), fromJson(it.value(), it.key()));
        return map;
    }
    if (value.isArray()) {
        QVariantList list;
        const QJsonArray array = value.toArray();
        for (const QJsonValue &item : array)
            list.append(fromJson(item, QString()));
        return list;
    }
    return value.toVariant();
}

QVariant Bridge::call(const QString &method, const QVariantMap &args)
{
    const QByteArray methodBytes = method.toUtf8();
    const QByteArray argsBytes = QJsonDocument::fromVariant(args).toJson(QJsonDocument::Compact);
    char *raw = WalletCall(const_cast<char *>(methodBytes.constData()),
                           const_cast<char *>(argsBytes.constData()));
    if (!raw)
        return QVariant();
    const QByteArray json(raw);
    WalletFree(raw);
    QJsonParseError error;
    const QJsonDocument doc = QJsonDocument::fromJson(json, &error);
    if (error.error == QJsonParseError::NoError) {
        if (doc.isArray())
            return fromJson(doc.array(), QString());
        if (doc.isObject())
            return fromJson(doc.object(), QString());
    }
    // QJsonDocument only accepts objects and arrays. Wrap a JSON primitive.
    const QJsonDocument wrapped = QJsonDocument::fromJson(QByteArray("[") + json + "]");
    if (wrapped.isArray() && !wrapped.array().isEmpty())
        return fromJson(wrapped.array().first(), QString());
    return QString::fromUtf8(json);
}

QByteArray Bridge::image(const QString &key, const QString &name, int w, int h)
{
    const QByteArray keyBytes = key.toUtf8();
    const QByteArray nameBytes = name.toUtf8();
    int len = 0;
    void *raw = WalletImage(const_cast<char *>(keyBytes.constData()),
                            const_cast<char *>(nameBytes.constData()), w, h, &len);
    if (!raw || len <= 0)
        return QByteArray();
    const QByteArray png(static_cast<const char *>(raw), len);
    WalletFree(raw);
    return png;
}
