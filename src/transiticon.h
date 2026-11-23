#ifndef TRANSITICON_H
#define TRANSITICON_H

#include <QColor>
#include <QQuickPaintedItem>

class TransitIcon : public QQuickPaintedItem
{
    Q_OBJECT
    Q_PROPERTY(QString transitType READ transitType WRITE setTransitType NOTIFY transitTypeChanged)
    Q_PROPERTY(QColor color READ color WRITE setColor NOTIFY transitTypeChanged)

public:
    explicit TransitIcon(QQuickItem *parent = nullptr);
    void paint(QPainter *painter) override;
    QString transitType() const { return m_type; }
    QColor color() const { return m_color; }
    void setTransitType(const QString &type);
    void setColor(const QColor &color);

signals:
    void transitTypeChanged();

private:
    QString m_type;
    QColor m_color = Qt::white;
};

#endif
