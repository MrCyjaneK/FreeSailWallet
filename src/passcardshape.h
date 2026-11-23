#ifndef PASSCARDSHAPE_H
#define PASSCARDSHAPE_H

#include <QColor>
#include <QPainterPath>
#include <QQuickPaintedItem>
#include <QVariantList>

class PassCardShape : public QQuickPaintedItem
{
    Q_OBJECT
    Q_PROPERTY(QColor color READ color WRITE setColor NOTIFY colorChanged)
    Q_PROPERTY(QColor strokeColor READ strokeColor WRITE setStrokeColor NOTIFY colorChanged)
    Q_PROPERTY(qreal strokeWidth READ strokeWidth WRITE setStrokeWidth NOTIFY colorChanged)
    Q_PROPERTY(qreal radius READ radius WRITE setRadius NOTIFY colorChanged)
    Q_PROPERTY(bool topNotch READ topNotch WRITE setTopNotch NOTIFY colorChanged)
    Q_PROPERTY(bool perforatedTop READ perforatedTop WRITE setPerforatedTop NOTIFY colorChanged)
    Q_PROPERTY(qreal perforationRadius READ perforationRadius WRITE setPerforationRadius NOTIFY colorChanged)
    Q_PROPERTY(qreal sideNotchY READ sideNotchY WRITE setSideNotchY NOTIFY colorChanged)
    Q_PROPERTY(qreal sideNotchRadius READ sideNotchRadius WRITE setSideNotchRadius NOTIFY colorChanged)
    Q_PROPERTY(QVariantList shadowOffsets READ shadowOffsets WRITE setShadowOffsets NOTIFY extentChanged)
    Q_PROPERTY(QVariantList shadowOpacities READ shadowOpacities WRITE setShadowOpacities NOTIFY colorChanged)
    Q_PROPERTY(qreal extent READ extent NOTIFY extentChanged)

public:
    explicit PassCardShape(QQuickItem *parent = nullptr);
    void paint(QPainter *painter) override;

    QColor color() const { return m_color; }
    QColor strokeColor() const { return m_stroke; }
    qreal strokeWidth() const { return m_strokeWidth; }
    qreal radius() const { return m_radius; }
    bool topNotch() const { return m_topNotch; }
    bool perforatedTop() const { return m_perforated; }
    qreal perforationRadius() const { return m_perforationRadius; }
    qreal sideNotchY() const { return m_sideNotchY; }
    qreal sideNotchRadius() const { return m_sideNotchRadius; }
    QVariantList shadowOffsets() const { return m_offsets; }
    QVariantList shadowOpacities() const { return m_opacities; }
    qreal extent() const;

    void setColor(const QColor &color);
    void setStrokeColor(const QColor &color);
    void setStrokeWidth(qreal width);
    void setRadius(qreal radius);
    void setTopNotch(bool notch);
    void setPerforatedTop(bool perforated);
    void setPerforationRadius(qreal radius);
    void setSideNotchY(qreal y);
    void setSideNotchRadius(qreal radius);
    void setShadowOffsets(const QVariantList &offsets);
    void setShadowOpacities(const QVariantList &opacities);

signals:
    void colorChanged();
    void extentChanged();

private:
    QPainterPath cardPath(const QRectF &rect) const;

    QColor m_color;
    QColor m_stroke;
    qreal m_strokeWidth = 1;
    qreal m_radius = 12;
    bool m_topNotch = false;
    bool m_perforated = false;
    qreal m_perforationRadius = 4;
    qreal m_sideNotchY = 0;
    qreal m_sideNotchRadius = 6;
    QVariantList m_offsets;
    QVariantList m_opacities;
};

#endif
