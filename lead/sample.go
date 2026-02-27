package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	oai "github.com/firebase/genkit/go/plugins/compat_oai"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/stealth"
	"github.com/openai/openai-go"
)

// getChromeWSURL 从 Chrome 调试端口获取 WebSocket URL
func getChromeWSURL(port string) (string, error) {
	resp, err := http.Get("http://127.0.0.1:" + port + "/json/version")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", err
	}
	return v.WebSocketDebuggerURL, nil
}

// Readability 提取的结果结构
type ArticleResult struct {
	Title   string `json:"title"`
	Excerpt string `json:"excerpt"` // 摘要
	Text    string `json:"text"`    // 纯文本正文
	URL     string `json:"url"`
}

// 核心抓取逻辑（使用 rod + stealth 绕过反爬）
func fetchWithReadability(ctx context.Context) ([]ArticleResult, error) {
	var url string
	var l *launcher.Launcher

	// 优先连接用户已启动的 Chrome（无自动化标识，最难被检测）
	// 用法：先关闭所有 Chrome，再运行（必须加 --user-data-dir 否则不监听 9222）:
	//   chrome.exe --remote-debugging-port=9222 --user-data-dir=%TEMP%\chrome-debug
	// 或执行 start-chrome-debug.ps1 / start-chrome-debug.bat
	url = "http://127.0.0.1:9222/devtools/browser"

	browser := rod.New().Timeout(90 * time.Second).ControlURL(url).MustConnect()
	defer browser.MustClose()
	if l != nil {
		defer l.Cleanup()
	}

	// stealth 注入（需在首次导航前）
	page := stealth.MustPage(browser)
	defer page.MustClose()

	var targetURLs []string

	// 1. 访问列表页，多策略提取详情链接
	listURL := "https://www.yfbzb.com/search/invitedBidSearch?defaultSearch=true&keyword=无纸化"
	page.MustNavigate(listURL).MustWaitLoad()
	page.MustWaitStable()
	time.Sleep(5 * time.Second) // 模拟人工等待，避免被识别为自动化

	extractJS := `() => {
		const base = window.location.origin;
		const isDetailLink = (href) => href && href.startsWith(base) && !href.includes('#') && href !== base && href !== base + '/';
		const dedupe = (arr) => arr.filter((v, i, a) => a.indexOf(v) === i);
		const take = (arr) => dedupe(arr).slice(0, 3);

		let urls = Array.from(document.querySelectorAll('a[href*="/inviteBid/detail/"]')).map(a => a.href).filter(isDetailLink);
		if (urls.length >= 1) return take(urls);

		urls = Array.from(document.querySelectorAll('table tbody tr td:first-child a, table tr td:first-child a')).map(a => a.href).filter(isDetailLink);
		if (urls.length >= 1) return take(urls);

		const main = document.querySelector('main, #content, .content, .list-content, [role="main"]') || document.body;
		const excludeContainers = document.querySelectorAll('header, footer, nav, aside, .footer, .header, .nav, .sidebar');
		const isExcluded = (el) => [].some.call(excludeContainers, c => c.contains(el));
		urls = Array.from(main.querySelectorAll('a')).filter(a => !isExcluded(a) && isDetailLink(a.href) && a.innerText.trim().length > 8 && /招标|采购|公告|项目|竞价|磋商|谈判/.test(a.innerText)).map(a => a.href);
		if (urls.length >= 1) return take(urls);

		urls = Array.from(document.querySelectorAll('a')).filter(a => isDetailLink(a.href) && a.innerText.trim().length > 10 && /招标|采购|公告/.test(a.innerText)).map(a => a.href);
		return take(urls);
	}`

	val, err := page.Eval(extractJS)
	if err != nil {
		return nil, fmt.Errorf("提取列表链接失败: %w", err)
	}

	// 解析 Eval 返回的 JSON 数组（gson.JSON）
	if !val.Value.Nil() {
		for _, v := range val.Value.Arr() {
			targetURLs = append(targetURLs, v.Str())
		}
	}

	fmt.Println("targetURLs: ", targetURLs)

	if len(targetURLs) == 0 {
		return nil, fmt.Errorf("无法识别列表链接")
	}

	var results []ArticleResult

	// 2. 遍历详情页，注入 Readability
	readabilityJS := `() => {
		return (async () => {
			if (typeof Readability === 'undefined') {
				const script = document.createElement('script');
				script.src = 'https://cdnjs.cloudflare.com/ajax/libs/readability/0.5.0/Readability.min.js';
				document.head.appendChild(script);
				await new Promise(r => script.onload = r);
			}
			const docClone = document.cloneNode(true);
			const article = new Readability(docClone).parse();
			if (!article) return null;
			return {
				title: article.title,
				excerpt: article.excerpt, 
				text: article.textContent.replace(/\s+/g, ' ').trim()
			};
		})();
	}`

	for _, url := range targetURLs {
		page.MustNavigate(url).MustWaitLoad()
		page.MustWaitStable()
		time.Sleep(1 * time.Second)

		val, err := page.Eval(readabilityJS)
		if err != nil {
			continue
		}

		var res ArticleResult
		if !val.Value.Nil() {
			val.Value.Unmarshal(&res)
		}
		if res.Title != "" || res.Text != "" {
			res.URL = url
			results = append(results, res)
		}
	}

	return results, nil
}

func main() {
	baseURL := "https://llm.thunisoft.com/v1"
	apiKey := "8d6dd05d-8081-4176-86f0-28ddac18d165"
	modelName := "DeepSeek-V3.2"
	temperature := 0.7

	ctx := context.Background()

	plugin := &oai.OpenAICompatible{
		Provider: "oai",
		APIKey:   apiKey,
		BaseURL:  baseURL,
	}
	g := genkit.Init(ctx, genkit.WithPlugins(plugin),
		genkit.WithDefaultModel("oai/"+modelName))

	yfbSmartTool := genkit.DefineTool(
		g,
		"yfb_smart_reader",
		"访问乙方宝，访问招标公告页面，从列表页提取最新的招标项目信息",
		func(tc *ai.ToolContext, input struct{ URL string }) ([]ArticleResult, error) {
			return fetchWithReadability(ctx)
		},
	)

	config := &openai.ChatCompletionNewParams{
		Temperature: openai.Float(temperature),
	}
	resp, err := genkit.Generate(ctx, g,
		ai.WithConfig(config),
		ai.WithPrompt("去乙方宝 https://www.yfbzb.com/search/invitedBidSearch?defaultSearch=true 看看最新的招标项目，总结一下重点。"),
		ai.WithTools(yfbSmartTool),
	)
	if err != nil {
		fmt.Printf("生成失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(resp.Text())
}
