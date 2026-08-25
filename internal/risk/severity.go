package risk

func SeverityRank(v string) int {
	switch v {
	case "高":
		return 3
	case "中":
		return 2
	case "低":
		return 1
	default:
		return 0
	}
}
