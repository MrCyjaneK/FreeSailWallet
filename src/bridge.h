#ifndef BRIDGE_H
#define BRIDGE_H

#include <QByteArray>
#include <QString>
#include <QVariant>
#include <QVariantMap>

class Bridge
{
public:
    static QVariant call(const QString &method, const QVariantMap &args = QVariantMap());
    static QByteArray image(const QString &key, const QString &name, int w, int h);
};

#endif
