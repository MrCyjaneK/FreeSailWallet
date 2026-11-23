#include "passcardshape.h"

#include <QPainter>
#include <QPainterPath>
#include <QtMath>

PassCardShape::PassCardShape(QQuickItem *parent)
    : QQuickPaintedItem(parent)
{
    setAntialiasing(true);
}

qreal PassCardShape::extent() const
{
    qreal maxOffset = 0;
    for (const QVariant &offset : m_offsets)
        maxOffset = qMax(maxOffset, offset.toReal());
    return maxOffset;
}

void PassCardShape::setColor(const QColor &color)
{
    if (m_color == color)
        return;
    m_color = color;
    emit colorChanged();
    update();
}

void PassCardShape::setStrokeColor(const QColor &color)
{
    if (m_stroke == color)
        return;
    m_stroke = color;
    emit colorChanged();
    update();
}

void PassCardShape::setStrokeWidth(qreal width)
{
    if (qFuzzyCompare(m_strokeWidth, width))
        return;
    m_strokeWidth = width;
    emit colorChanged();
    update();
}

void PassCardShape::setRadius(qreal radius)
{
    if (qFuzzyCompare(m_radius, radius))
        return;
    m_radius = radius;
    emit colorChanged();
    update();
}

void PassCardShape::setTopNotch(bool notch)
{
    if (m_topNotch == notch)
        return;
    m_topNotch = notch;
    emit colorChanged();
    update();
}

void PassCardShape::setPerforatedTop(bool perforated)
{
    if (m_perforated == perforated)
        return;
    m_perforated = perforated;
    emit colorChanged();
    update();
}

void PassCardShape::setPerforationRadius(qreal radius)
{
    if (qFuzzyCompare(m_perforationRadius, radius))
        return;
    m_perforationRadius = radius;
    emit colorChanged();
    update();
}

void PassCardShape::setSideNotchY(qreal y)
{
    if (qFuzzyCompare(m_sideNotchY, y))
        return;
    m_sideNotchY = y;
    emit colorChanged();
    update();
}

void PassCardShape::setSideNotchRadius(qreal radius)
{
    if (qFuzzyCompare(m_sideNotchRadius, radius))
        return;
    m_sideNotchRadius = radius;
    emit colorChanged();
    update();
}

void PassCardShape::setShadowOffsets(const QVariantList &offsets)
{
    if (m_offsets == offsets)
        return;
    m_offsets = offsets;
    emit extentChanged();
    update();
}

void PassCardShape::setShadowOpacities(const QVariantList &opacities)
{
    if (m_opacities == opacities)
        return;
    m_opacities = opacities;
    emit colorChanged();
    update();
}

QPainterPath PassCardShape::cardPath(const QRectF &rect) const
{
    QPainterPath path;
    path.addRoundedRect(rect, m_radius, m_radius);
    if (m_topNotch) {
        const qreal notch = qMax<qreal>(m_radius, 14);
        QPainterPath cut;
        cut.addEllipse(QPointF(rect.center().x(), rect.top()), notch, notch * 0.72);
        path = path.subtracted(cut);
    }
    if (m_perforated && m_perforationRadius > 0) {
        const qreal step = m_perforationRadius * 3.2;
        for (qreal x = rect.left() + step; x < rect.right() - m_perforationRadius; x += step) {
            QPainterPath hole;
            hole.addEllipse(QPointF(x, rect.top()), m_perforationRadius, m_perforationRadius);
            path = path.subtracted(hole);
        }
    }
    if (m_sideNotchRadius > 0 && m_sideNotchY > 0) {
        const qreal y = rect.top() + m_sideNotchY;
        QPainterPath left;
        left.addEllipse(QPointF(rect.left(), y), m_sideNotchRadius, m_sideNotchRadius);
        QPainterPath right;
        right.addEllipse(QPointF(rect.right(), y), m_sideNotchRadius, m_sideNotchRadius);
        path = path.subtracted(left).subtracted(right);
    }
    return path;
}

void PassCardShape::paint(QPainter *painter)
{
    painter->setRenderHint(QPainter::Antialiasing);
    const qreal extra = extent();
    const QRectF card(0, extra, width(), qMax<qreal>(0, height() - extra));
    if (card.height() < 1 || card.width() < 1)
        return;
    for (int i = m_offsets.size() - 1; i >= 0; --i) {
        const qreal offset = m_offsets.at(i).toReal();
        qreal opacity = 0.08;
        if (i < m_opacities.size())
            opacity = m_opacities.at(i).toReal();
        painter->setPen(Qt::NoPen);
        painter->setBrush(QColor(0, 0, 0, int(qBound<qreal>(0, opacity, 1) * 255)));
        painter->drawPath(cardPath(card.translated(0, -offset)));
    }
    painter->setBrush(m_color);
    if (m_strokeWidth > 0 && m_stroke.alpha() > 0) {
        QPen pen(m_stroke);
        pen.setWidthF(m_strokeWidth);
        painter->setPen(pen);
    } else {
        painter->setPen(Qt::NoPen);
    }
    painter->drawPath(cardPath(card));
}
