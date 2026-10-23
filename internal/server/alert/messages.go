package alert

import (
	"fmt"
	"math"
	"time"
)

type texts struct {
	firing, resolved string
	offline          string
	online           string
	over             string // metric, value, threshold, duration
	normal           string // metric, value
	traffic          string // used, limit, percent
	expiring         string // date, days
	expired          string // date
	metric           map[string]string
	duration         func(time.Duration) string
}

var lang = map[string]texts{
	"en": {
		firing: "Alert", resolved: "Resolved",
		offline:  "Node %s has been offline for %s",
		online:   "Node %s is back online",
		over:     "Node %s: %s usage %.1f%% is above %.0f%% (for %s)",
		normal:   "Node %s: %s usage is back to normal (%.1f%%)",
		traffic:  "Node %s used %s of %s traffic this period (%.1f%%)",
		expiring: "Node %s expires on %s (%d days left)",
		expired:  "Node %s expired on %s",
		metric:   map[string]string{"cpu": "CPU", "mem": "memory", "disk": "disk"},
		duration: func(d time.Duration) string { return d.Round(time.Second).String() },
	},
	"zh": {
		firing: "告警", resolved: "恢复",
		offline:  "节点 %s 已离线 %s",
		online:   "节点 %s 已恢复在线",
		over:     "节点 %s 的%s使用率 %.1f%% 超过阈值 %.0f%%（持续 %s）",
		normal:   "节点 %s 的%s使用率已恢复正常（%.1f%%）",
		traffic:  "节点 %s 本周期已用流量 %s / %s（%.1f%%）",
		expiring: "节点 %s 将于 %s 到期（剩余 %d 天）",
		expired:  "节点 %s 已于 %s 到期",
		metric:   map[string]string{"cpu": "CPU", "mem": "内存", "disk": "磁盘"},
		duration: zhDuration,
	},
}

func textsFor(code string) texts {
	if t, ok := lang[code]; ok {
		return t
	}
	return lang["en"]
}

func zhDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h, m, s := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	switch {
	case h > 0:
		return fmt.Sprintf("%d 小时 %d 分", h, m)
	case m > 0:
		return fmt.Sprintf("%d 分 %d 秒", m, s)
	default:
		return fmt.Sprintf("%d 秒", s)
	}
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	exp := int(math.Log(float64(b)) / math.Log(unit))
	exp = min(exp, 6)
	return fmt.Sprintf("%.2f %ciB", float64(b)/math.Pow(unit, float64(exp)), "KMGTPE"[exp-1])
}
