.pragma library
function _channel(c) {
    return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
}
function _rgb(color) {
    if (color && typeof color.r === "number")
        return [color.r, color.g, color.b]
    var hex = String(color)
    if (hex.charAt(0) === "#") {
        var h = hex.substring(1)
        if (h.length === 3)
            h = h.charAt(0) + h.charAt(0) + h.charAt(1) + h.charAt(1) + h.charAt(2) + h.charAt(2)
        else if (h.length === 8)
            h = h.substring(2)
        var n = parseInt(h.substring(0, 6), 16)
        if (!isNaN(n))
            return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255]
    }
    return [0, 0, 0]
}
function luminance(color) {
    var rgb = _rgb(color)
    return 0.2126 * _channel(rgb[0]) + 0.7152 * _channel(rgb[1]) + 0.0722 * _channel(rgb[2])
}
function contrast(a, b) {
    var hi = Math.max(luminance(a), luminance(b))
    var lo = Math.min(luminance(a), luminance(b))
    return (hi + 0.05) / (lo + 0.05)
}
function readable(ink, paper, dark, light) {
    if (contrast(ink, paper) >= 3)
        return ink
    if (!dark)
        dark = "#1a1a1a"
    if (!light)
        light = "#ffffff"
    return contrast(dark, paper) >= contrast(light, paper) ? dark : light
}
