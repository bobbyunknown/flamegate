package admin

func normalizeBudgetPeriod(period string) (string, bool) {
	period = defaultStr(period, "monthly")
	switch period {
	case "daily", "weekly", "monthly", "total":
		return period, true
	default:
		return "", false
	}
}

// adminBudgetStatus returns all budgets enriched with current-period spend data.

// ---- usage ------------------------------------------------------------------
