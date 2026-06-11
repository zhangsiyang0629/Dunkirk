package pipeline

import (
	"context"
	"dunkirk/internal/config"
	"dunkirk/internal/kb"
	"testing"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/redis/go-redis/v9"
)

func TestScriptChain(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkChatModel,
	})
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

	streamingMode := &streamingModel{BaseChatModel: cm}
	plainChain := newPlainScriptChain(ctx, streamingMode)

	runnable, err := plainChain.Compile(ctx)

	systemPrompt := `你是专业的有声读物编剧，擅长将枯燥的历史故事改编成7岁小朋友爱听的生动故事。

## 改编原则（必须遵守）
1. **拒绝复读机**：不要简单换一种说法复述原文。请在不违背历史情节的前提下，
   大胆补充人物的对话、心理活动、动作细节。
2. **丰富感官描写**：加入声音（啊呀呀！叮叮当当！咚咚咚！）、表情（瞪圆了眼睛、咧开嘴笑）、
   动作细节（握紧了拳头、擦了擦汗）。
3. **扩展关键场景**：原文一句话带过的战斗、冲突，你要展开成3-5句。
4. **口语化表达**：用"哇塞"、"哎呀"、"太厉害了"等口语。多用拟声词和感叹词。
5. **用词规范**：小孩听不懂的词要解释。比如"檄文"改成"一封很厉害的信"。
6. **大胆扩展**：你的输出长度**至少**是原文的 **1.5倍**。
7. **上文承接**：当用户没有提供上一回结尾时，不用机械强行进行衔接

## 节奏感原则（非常重要）
1. **拟声词只用在关键时刻**：只在战斗、冲突、情绪爆发等高潮场景使用拟声词。平淡的叙述（走路、说话、思考）不要加。
2. **高潮场景详细写，过渡场景简单写**：
   - 高潮场景（战斗、对决、重要对话）：进行适当的扩写。
   - 过渡场景（行军、铺垫、日常）：简单带过即可，不要过度渲染。
3. **避免每句都加感叹词**："啊"、"呀"、"哇塞" 这类感叹词，只在人物情绪波动时使用，不要句句都加。

## 必须在每个场景中做到的三件事（逐句检查）
1. **每 100 字至少 1 个拟声词**：战斗用“咚咚咚”“嗖嗖嗖”“轰隆隆”“噼里啪啦”等等，
   人物动作用“啪嗒啪嗒”“咕噜咕噜”“呼哧呼哧”等等。
2. **每个关键人物至少 1 次内心独白**：被骗时想什么？中计后想什么？
   用引号把心里话写出来，比如“完了完了，上当了！”
3. **每个高潮瞬间至少 1 个特写镜头**：不要只写“他冲过去”，要写
   “他咬紧了牙关，眼睛瞪得像铜铃，汗珠从额头滚下来，大吼一声冲了过去”。

## 改写示例
原文："张飞大喝一声，冲入敌阵。"
合格改写："张飞深吸一口气，用足全身力气大喝一声：'啊呀呀呀呀呀呀！'
   他瞪着铜铃大的眼睛，胡子一根根竖起来，手提丈八蛇矛，像一头下山猛虎，
   冲入敌阵！"
`

	content := `第九十七回　讨魏国武侯再上表　破曹兵姜维诈献书
其人曰：“小人乃姜伯约心腹人也。蒙本官 遣送密书。”真曰：“书安在？”其人于贴 肉衣内取出呈上。真拆视曰：“罪将姜维百拜， 书呈大都督曹麾下：维念世食魏禄，忝守边 城；叨窃厚恩，无门补报。昨日误遭诸葛亮 之计，陷身于巅崖之中。思念旧国，何日忘 之！今幸蜀兵西出，诸葛亮甚不相疑。赖都 督亲提大兵而来：如遇敌人，可以诈败；维 当在后，以举火为号，先烧蜀人粮草，却以 大兵翻身掩之，则诸葛亮可擒也。非敢立功 报国，实欲自赎前罪。倘蒙照察，速赐来命。” 曹真看毕，大喜曰：“天使吾成功也！”遂 重赏来人，便令回报，依期会合。真唤费耀 商议曰：“今姜维暗献密书，令吾如此如此。” 耀曰：“诸葛亮多谋，姜维智广，或者是诸 葛亮所使，恐其中有诈。”真曰：“他原是 魏人，不得已而降蜀，又何疑乎？”耀曰：“都 督不可轻去，只守定本寨。某愿引一军接应 姜维。如成，功尽归都督；倘有奸计，某自 支当。”真大喜，遂令费耀引五万兵，望斜 谷而进。行了两三程，屯下军马，令人哨探。

当日申时分，回报：“斜谷道中，有蜀兵来也。” 耀忙催兵进。蜀兵未及交战先退。耀引兵追 之，蜀兵又来。方欲对阵，蜀兵又退：如此 者三次，俄延至次日申时分。魏军一日一夜， 不曾敢歇，只恐蜀兵攻击。方欲屯军造饭， 忽然四面喊声大震，鼓角齐鸣，蜀兵漫山遍 野而来。门旗开处，闪出一辆四轮车，孔明 端坐其中，令人请魏军主将答话。耀纵马而 出，遥见孔明，心中暗喜，回顾左右曰：“如 蜀兵掩至，便退后走。若见山后火起，却回 身杀去，自有兵来相应。”分付毕，跃马出 呼曰：“前者败将，今何敢又来！”孔明曰： “唤汝曹真来答话！”耀骂曰：“曹都督乃 金枝玉叶，安肯与反贼相见耶！”孔明大怒， 把羽扇一招，左有马岱，右有张嶷，两路兵 冲出。魏兵便退。行不到三十里，望见蜀兵 背后火起，喊声不绝。费耀只道号火，便回 身杀来。蜀兵齐退。耀提刀在前，只望喊处 追赶。将次近火，山路中鼓角喧天、喊声震地， 两军杀出：左有关兴，右有张苞。山上矢石 如雨，往下射来。魏兵大败。费耀知是中计， 急退军望山谷中而走，人马困乏。背后关兴 引生力军赶来，魏兵自相践踏及落涧身死者， 不知其数。`

	runeLen := len([]rune(content))
	t.Logf("runeLen = %d\n", runeLen)

	input := map[string]any{
		"content":       content,
		"topic":         "第九十七回　讨魏国武侯再上表　破曹兵姜维诈献书",
		"style":         "适合7岁小朋友",
		"duration_min":  1,
		"rune_len":      runeLen,
		"use_ssml":      false,
		"prev_ending":   "",
		"system_prompt": systemPrompt,
	}

	output, err := runnable.Invoke(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(output.Content)
}

func TestWholeChapterScriptChain(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkChatModel,
	})
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

	streamingMode := &streamingModel{BaseChatModel: cm}
	plainChain := newPlainScriptChain(ctx, streamingMode)

	runnable, err := plainChain.Compile(ctx)
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:          cfg.RedisAddr,
		Protocol:      2,
		UnstableResp3: true,
	})

	knowledgeBase, err := kb.New(ctx, cfg, rdb)
	if err != nil {
		t.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()

	content, err := knowledgeBase.GetChapterSegments(ctx, "75eb95dc", "第九回　除暴凶吕布助司徒　犯长安李傕听贾诩")
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

	systemPrompt := `你是专业的有声读物编剧，擅长将枯燥的历史故事改编成7岁小朋友爱听的生动故事。

## 改编原则（必须遵守）
1. **拒绝复读机**：不要简单换一种说法复述原文。请在不违背历史情节的前提下，
   大胆补充人物的对话、心理活动、动作细节。
2. **丰富感官描写**：加入声音（啊呀呀！叮叮当当！咚咚咚！）、表情（瞪圆了眼睛、咧开嘴笑）、
   动作细节（握紧了拳头、擦了擦汗）。
3. **扩展关键场景**：原文一句话带过的战斗、冲突，你要展开成3-5句。
4. **口语化表达**：用"哇塞"、"哎呀"、"太厉害了"等口语。多用拟声词和感叹词。
5. **用词规范**：小孩听不懂的词要解释。比如"檄文"改成"一封很厉害的信"。
6. **大胆扩展**：你的输出长度**至少**是原文的 **1.5倍**。
7. **上文承接**：当用户没有提供上一回结尾时，不用机械强行进行衔接

## 节奏感原则（非常重要）
1. **拟声词只用在关键时刻**：只在战斗、冲突、情绪爆发等高潮场景使用拟声词。平淡的叙述（走路、说话、思考）不要加。
2. **高潮场景详细写，过渡场景简单写**：
   - 高潮场景（战斗、对决、重要对话）：进行适当的扩写。
   - 过渡场景（行军、铺垫、日常）：简单带过即可，不要过度渲染。
3. **避免每句都加感叹词**："啊"、"呀"、"哇塞" 这类感叹词，只在人物情绪波动时使用，不要句句都加。

## 必须在每个场景中做到的三件事（逐句检查）
1. **每 100 字至少 1 个拟声词**：战斗用“咚咚咚”“嗖嗖嗖”“轰隆隆”“噼里啪啦”等等，
   人物动作用“啪嗒啪嗒”“咕噜咕噜”“呼哧呼哧”等等。
2. **每个关键人物至少 1 次内心独白**：被骗时想什么？中计后想什么？
   用引号把心里话写出来，比如“完了完了，上当了！”
3. **每个高潮瞬间至少 1 个特写镜头**：不要只写“他冲过去”，要写
   “他咬紧了牙关，眼睛瞪得像铜铃，汗珠从额头滚下来，大吼一声冲了过去”。

## 改写示例
原文："张飞大喝一声，冲入敌阵。"
合格改写："张飞深吸一口气，用足全身力气大喝一声：'啊呀呀呀呀呀呀！'
   他瞪着铜铃大的眼睛，胡子一根根竖起来，手提丈八蛇矛，像一头下山猛虎，
   冲入敌阵！"
`

	runeLen := len([]rune(content))
	t.Logf("runeLen = %d\n", runeLen)

	input := map[string]any{
		"content":       content,
		"topic":         "第九十七回　讨魏国武侯再上表　破曹兵姜维诈献书",
		"style":         "适合7岁小朋友",
		"duration_min":  1,
		"rune_len":      runeLen,
		"use_ssml":      false,
		"prev_ending":   "",
		"system_prompt": systemPrompt,
	}

	output, err := runnable.Invoke(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(output.Content)
}

func TestWholeChapterSegScriptChain(t *testing.T) {
	ctx := context.Background()
	cfg := config.Load()
	cm, err := ark.NewChatModel(ctx, &ark.ChatModelConfig{
		APIKey: cfg.ArkAPIKey,
		Model:  cfg.ArkChatModel,
	})
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

	streamingMode := &streamingModel{BaseChatModel: cm}
	plainChain := newSegmentedScriptChain(ctx, streamingMode)

	runnable, err := plainChain.Compile(ctx)
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:          cfg.RedisAddr,
		Protocol:      2,
		UnstableResp3: true,
	})

	knowledgeBase, err := kb.New(ctx, cfg, rdb)
	if err != nil {
		t.Fatalf("init kb: %v", err)
	}
	defer knowledgeBase.Close()

	content, err := knowledgeBase.GetChapterSegments(ctx, "75eb95dc", "第九回　除暴凶吕布助司徒　犯长安李傕听贾诩")
	if err != nil {
		t.Fatalf("new chat model: %v", err)
	}

	runeLen := len([]rune(content))
	t.Logf("runeLen = %d\n", runeLen)

	input := map[string]any{
		"content":       content,
		"topic":         "第九回　除暴凶吕布助司徒　犯长安李傕听贾诩",
		"style":         "适合7岁小朋友",
		"duration_min":  0,
		"rune_len":      0,
		"use_ssml":      false,
		"prev_ending":   "",
		"system_prompt": "",
	}

	output, err := runnable.Invoke(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(output.Content)
}
