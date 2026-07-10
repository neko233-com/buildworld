package engine

import (
	"fmt"
	"strings"
)

// GenerateBadge 生成 shields.io 风格的 SVG 徽章。
// status 决定颜色：success=green, failed=red, running=blue, 其他=gray。
func GenerateBadge(status, label string) []byte {
	if label == "" {
		label = "build"
	}
	color, text := badgeColor(status)
	widthLabel := 10 + 6*len(label)
	widthStatus := 10 + 6*len(text)
	widthTotal := widthLabel + widthStatus
	// 使用 simple shields.io 风格 SVG
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20">
<linearGradient id="b" x2="0" y2="100%%">
<stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
<stop offset="1" stop-opacity=".1"/>
</linearGradient>
<mask id="a"><rect width="%d" height="20" rx="3" fill="#fff"/></mask>
<g mask="url(#a)">
<path fill="#555" d="M0 0h%dv20H0z"/>
<path fill="%s" d="M%d 0h%dv20H%dz"/>
<path fill="url(#b)" d="M0 0h%dv20H0z"/>
</g>
<g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
<text x="%d" y="15">%s</text>
<text x="%d" y="15">%s</text>
</g>
</svg>`,
		widthTotal, widthTotal,
		widthLabel,
		color, widthLabel, widthStatus, widthLabel,
		widthTotal,
		widthLabel/2, escapeXML(label),
		widthLabel+widthStatus/2, escapeXML(text),
	)
	return []byte(svg)
}

func badgeColor(status string) (color, text string) {
	switch strings.ToLower(status) {
	case "success", "passed", "ok":
		return "#4c1", "success"
	case "failed", "failure", "error":
		return "#e05d44", "failed"
	case "running", "in-progress":
		return "#007ec6", "running"
	case "pending", "queued":
		return "#dfb317", "pending"
	case "cancelled", "canceled":
		return "#9f9f9f", "cancelled"
	default:
		return "#9f9f9f", status
	}
}

func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;")
	return r.Replace(s)
}
