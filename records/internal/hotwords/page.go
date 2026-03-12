package hotwords

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"
)

// PageData 供前端/API 使用的数据结构（按时间窗口）
type PageData struct {
	RunTime         time.Time     `json:"run_time"`
	Windows         []WindowData `json:"windows"`
	CategoryLabels  map[string]string `json:"category_labels,omitempty"`
}

// WindowData 单个时间窗口的统计
type WindowData struct {
	Days   int             `json:"days"`
	Title  string          `json:"title"`
	Groups []CategoryGroup `json:"groups"`
}

// CategoryGroup 某分类下的热词列表
type CategoryGroup struct {
	Category string     `json:"category"`
	Label    string     `json:"label"`
	Items    []TermFreq `json:"items"`
}

// TermFreq 关键词与频次
type TermFreq struct {
	Term      string `json:"term"`
	Frequency int    `json:"frequency"`
	Rank      int    `json:"rank"`
}

var categoryLabels = map[string]string{
	CategoryProducts:             "产品热词榜",
	CategoryBusinessRequirements: "客户需求热词",
	CategoryPainPoints:           "客户痛点",
	CategoryTransactionFriction:  "成交阻力",
}

// BuildPageData 从数据库取最近一次各时间窗口统计，组装为页面数据
func BuildPageData(ctx context.Context, db *sqlx.DB) (*PageData, error) {
	repo := NewRepo(db)
	var runTime time.Time
	windows := make([]WindowData, 0, len(TimeWindowDays))
	for _, days := range TimeWindowDays {
		rows, err := repo.LatestStatsByWindow(ctx, days)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			windows = append(windows, WindowData{Days: days, Title: fmt.Sprintf("最近%d天", days), Groups: nil})
			continue
		}
		if runTime.IsZero() || rows[0].RunTime.After(runTime) {
			runTime = rows[0].RunTime
		}
		byCat := make(map[string][]TermFreq)
		for _, r := range rows {
			byCat[r.Category] = append(byCat[r.Category], TermFreq{Term: r.Term, Frequency: r.Frequency, Rank: r.Rank})
		}
		catOrder := []string{CategoryProducts, CategoryBusinessRequirements, CategoryPainPoints, CategoryTransactionFriction}
		var groups []CategoryGroup
		for _, cat := range catOrder {
			items := byCat[cat]
			sort.Slice(items, func(i, j int) bool { return items[i].Rank < items[j].Rank })
			groups = append(groups, CategoryGroup{
				Category: cat,
				Label:    categoryLabels[cat],
				Items:    items,
			})
		}
		windows = append(windows, WindowData{
			Days:   days,
			Title:  fmt.Sprintf("最近%d天", days),
			Groups: groups,
		})
	}
	return &PageData{
		RunTime:         runTime,
		Windows:         windows,
		CategoryLabels:  categoryLabels,
	}, nil
}

// BuildPageDataByDate 取指定日期当天的热词统计（该日最后一次 run_time 的数据）；若该日无统计则返回空数据
func BuildPageDataByDate(ctx context.Context, db *sqlx.DB, date time.Time) (*PageData, error) {
	repo := NewRepo(db)
	runTime, err := repo.RunTimeForDate(ctx, date)
	if err != nil {
		return nil, err
	}
	if runTime.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		return &PageData{RunTime: date, Windows: []WindowData{}, CategoryLabels: categoryLabels}, nil
	}
	rows, err := repo.StatsByRunTime(ctx, runTime)
	if err != nil {
		return nil, err
	}
	byWindow := make(map[int]map[string][]TermFreq)
	for _, r := range rows {
		if byWindow[r.TimeWindowDays] == nil {
			byWindow[r.TimeWindowDays] = make(map[string][]TermFreq)
		}
		byWindow[r.TimeWindowDays][r.Category] = append(byWindow[r.TimeWindowDays][r.Category], TermFreq{Term: r.Term, Frequency: r.Frequency, Rank: r.Rank})
	}
	catOrder := []string{CategoryProducts, CategoryBusinessRequirements, CategoryPainPoints, CategoryTransactionFriction}
	var windows []WindowData
	for _, days := range TimeWindowDays {
		byCat := byWindow[days]
		var groups []CategoryGroup
		for _, cat := range catOrder {
			items := byCat[cat]
			sort.Slice(items, func(i, j int) bool { return items[i].Rank < items[j].Rank })
			groups = append(groups, CategoryGroup{Category: cat, Label: categoryLabels[cat], Items: items})
		}
		windows = append(windows, WindowData{Days: days, Title: fmt.Sprintf("最近%d天", days), Groups: groups})
	}
	return &PageData{RunTime: runTime, Windows: windows, CategoryLabels: categoryLabels}, nil
}
