#include "transiticon.h"

#include <QPainter>
#include <QPainterPath>

TransitIcon::TransitIcon(QQuickItem *parent)
    : QQuickPaintedItem(parent)
{
    setAntialiasing(true);
}

void TransitIcon::setTransitType(const QString &type)
{
    if (m_type == type)
        return;
    m_type = type;
    emit transitTypeChanged();
    update();
}

void TransitIcon::setColor(const QColor &color)
{
    if (m_color == color)
        return;
    m_color = color;
    emit transitTypeChanged();
    update();
}

void TransitIcon::paint(QPainter *painter)
{
    painter->setRenderHint(QPainter::Antialiasing);
    painter->setPen(Qt::NoPen);
    painter->setBrush(m_color);
    const QRectF box(0, 0, width(), height());
    const QString type = m_type.toLower();
    if (type.contains(QLatin1String("air"))) {
        QPainterPath plane;
        plane.moveTo(box.center().x(), box.top() + box.height() * 0.12);
        plane.lineTo(box.right() - box.width() * 0.08, box.center().y());
        plane.lineTo(box.center().x(), box.center().y() + box.height() * 0.08);
        plane.lineTo(box.left() + box.width() * 0.08, box.center().y());
        plane.closeSubpath();
        painter->drawPath(plane);
        painter->drawRoundedRect(QRectF(box.center().x() - box.width() * 0.06, box.top() + box.height() * 0.28,
                                        box.width() * 0.12, box.height() * 0.5), 2, 2);
        return;
    }
    if (type.contains(QLatin1String("train"))) {
        painter->drawRoundedRect(QRectF(box.left() + box.width() * 0.18, box.top() + box.height() * 0.12,
                                        box.width() * 0.64, box.height() * 0.62), 4, 4);
        painter->setBrush(m_color.darker(140));
        painter->drawEllipse(QPointF(box.left() + box.width() * 0.32, box.bottom() - box.height() * 0.16),
                             box.width() * 0.1, box.width() * 0.1);
        painter->drawEllipse(QPointF(box.right() - box.width() * 0.32, box.bottom() - box.height() * 0.16),
                             box.width() * 0.1, box.width() * 0.1);
        return;
    }
    if (type.contains(QLatin1String("bus"))) {
        painter->drawRoundedRect(box.adjusted(box.width() * 0.12, box.height() * 0.16,
                                              -box.width() * 0.12, -box.height() * 0.22), 3, 3);
        painter->setBrush(m_color.darker(150));
        painter->drawEllipse(QPointF(box.left() + box.width() * 0.3, box.bottom() - box.height() * 0.16),
                             box.width() * 0.08, box.width() * 0.08);
        painter->drawEllipse(QPointF(box.right() - box.width() * 0.3, box.bottom() - box.height() * 0.16),
                             box.width() * 0.08, box.width() * 0.08);
        return;
    }
    if (type.contains(QLatin1String("boat")) || type.contains(QLatin1String("ferry"))) {
        QPainterPath hull;
        hull.moveTo(box.left() + box.width() * 0.1, box.center().y());
        hull.lineTo(box.right() - box.width() * 0.1, box.center().y());
        hull.lineTo(box.right() - box.width() * 0.22, box.bottom() - box.height() * 0.18);
        hull.lineTo(box.left() + box.width() * 0.22, box.bottom() - box.height() * 0.18);
        hull.closeSubpath();
        painter->drawPath(hull);
        painter->drawRect(QRectF(box.center().x() - 1, box.top() + box.height() * 0.12, 2, box.height() * 0.4));
        return;
    }
    painter->drawRoundedRect(box.adjusted(box.width() * 0.2, box.height() * 0.2,
                                          -box.width() * 0.2, -box.height() * 0.2), 3, 3);
}
