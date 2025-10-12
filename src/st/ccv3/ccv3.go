package ccv3

// Role 定义消息角色
type Role int

const (
	RoleSystem = iota
	RoleUser
	RoleAssistant
)

// CharacterCard 角色卡V3的顶层结构
type CharacterCard struct {
	Spec        string            `json:"spec"`         // 规范标识，必须为 "chara_card_v3"
	SpecVersion string            `json:"spec_version"` // 规范版本，必须为 "3.0"
	Data        CharacterCardData `json:"data"`         // 包含角色卡核心数据的对象
}

// CharacterCardData 包含了角色的所有核心信息
type CharacterCardData struct {
	Name                    string                 `json:"name"`                      // 角色名称
	Description             string                 `json:"description"`               // 角色描述
	Tags                    []string               `json:"tags"`                      // 标签数组
	Creator                 string                 `json:"creator"`                   // 创建者
	CharacterVersion        string                 `json:"character_version"`         // 角色版本
	MesExample              string                 `json:"mes_example"`               // 聊天示例
	Extensions              CharacterCardExtension `json:"extensions"`                // 扩展数据，用于存储特定应用的数据
	SystemPrompt            string                 `json:"system_prompt"`             // 系统提示
	PostHistoryInstructions string                 `json:"post_history_instructions"` // 后历史指令
	FirstMes                string                 `json:"first_mes"`                 // 首条消息（默认问候语）
	AlternateGreetings      []string               `json:"alternate_greetings"`       // 备选问候语
	Personality             string                 `json:"personality"`               // 角色性格
	Scenario                string                 `json:"scenario"`                  // 场景设定

	CreatorNotes  string    `json:"creator_notes"`            // 创建者笔记 (作为 'en' 语言的默认笔记)
	CharacterBook *Lorebook `json:"character_book,omitempty"` // 角色设定集 (可选)

	Assets                   []Asset           `json:"assets,omitempty"`                     // 资源列表 (可选)
	Nickname                 string            `json:"nickname,omitempty"`                   // 角色昵称 (可选, 用于替换 {{char}})
	CreatorNotesMultilingual map[string]string `json:"creator_notes_multilingual,omitempty"` // 多语言创建者笔记 (可选)
	Source                   []string          `json:"source,omitempty"`                     // 角色卡来源 (ID或URL, 可选)
	GroupOnlyGreetings       []string          `json:"group_only_greetings"`                 // 仅群聊使用的问候语
	CreationDate             int64             `json:"creation_date,omitempty"`              // 创建日期 (Unix时间戳, 秒, 可选)
	ModificationDate         int64             `json:"modification_date,omitempty"`          // 修改日期 (Unix时间戳, 秒, 可选)
}

type CharacterCardExtension struct {
	DepthPrompt CharacterCardDepthPrompt `json:"depth_prompt,omitempty"` // 深度提示 (可选)
}

type CharacterCardDepthPrompt struct {
	Prompt string `json:"prompt"` // 深度提示内容
	Depth  int    `json:"depth"`  // 深度值
	Role   string `json:"role"`   // 角色 system, user, assistant
}

// Asset 定义一个与角色关联的资源
type Asset struct {
	Type string `json:"type"` // 资源类型 (如: icon, background, emotion)
	URI  string `json:"uri"`  // 资源URI (URL, base64, embeded://, ccdefault:)
	Name string `json:"name"` // 资源名称 (如: main, happy)
	Ext  string `json:"ext"`  // 资源文件扩展名 (如: png, webp)
}

// StandaloneLorebook 是独立导入导出时使用的设定集文件结构
type StandaloneLorebook struct {
	Spec string   `json:"spec"` // 规范标识，应为 "lorebook_v3"
	Data Lorebook `json:"data"` // 设定集核心数据
}

// Lorebook 定义的一个角色设定集
type Lorebook struct {
	Name              string                 `json:"name,omitempty"`               // 设定集名称 (可选)
	Description       string                 `json:"description,omitempty"`        // 设定集描述 (可选)
	ScanDepth         int                    `json:"scan_depth,omitempty"`         // 扫描深度 (最近N条消息, 可选)
	TokenBudget       int                    `json:"token_budget,omitempty"`       // Token预算 (可选)
	RecursiveScanning bool                   `json:"recursive_scanning,omitempty"` // 是否递归扫描 (可选)
	Extensions        map[string]interface{} `json:"extensions"`                   // 扩展数据
	Entries           []LorebookEntry        `json:"entries"`                      // 设定集条目数组
}

// LorebookEntry 定义设定集中的单个条目
type LorebookEntry struct {
	Keys           []string               `json:"keys"`                     // 触发关键词
	Content        string                 `json:"content"`                  // 注入的内容
	Extensions     LorebookEntryExtension `json:"extensions"`               // 扩展数据
	Enabled        bool                   `json:"enabled"`                  // 是否启用
	InsertionOrder int                    `json:"insertion_order"`          // 插入顺序
	CaseSensitive  bool                   `json:"case_sensitive,omitempty"` // 是否大小写敏感 (可选)
	UseRegex       bool                   `json:"use_regex"`                // 关键词是否使用正则表达式
	Constant       bool                   `json:"constant,omitempty"`       // 是否为常量条目 (无条件激活, 可选)

	Name          string      `json:"name,omitempty"`           // 条目名称 (可选)
	Priority      int         `json:"priority,omitempty"`       // 优先级 (可选)
	ID            interface{} `json:"id,omitempty"`             // 条目ID (可选, 字符串或数字)
	Comment       string      `json:"comment,omitempty"`        // 注释 (可选)
	Selective     bool        `json:"selective,omitempty"`      // 是否启用选择性激活 (与secondary_keys配合, 可选)
	SecondaryKeys []string    `json:"secondary_keys,omitempty"` // 第二触发关键词 (可选)
	Position      string      `json:"position,omitempty"`       // 注入位置 (无效)
}

// LorebookEntryExtension 定义设定集条目的扩展字段
type LorebookEntryExtension struct {
	Position                  LorebookInsertionPosition `json:"position,omitempty"`
	ExcludeRecursion          bool                      `json:"exclude_recursion,omitempty"`
	DisplayIndex              int                       `json:"display_index,omitempty"`
	Probability               int                       `json:"probability,omitempty"`
	UseProbability            bool                      `json:"useProbability,omitempty"`
	Depth                     int                       `json:"depth,omitempty"`
	SelectiveLogic            int                       `json:"selectiveLogic,omitempty"`
	Group                     string                    `json:"group,omitempty"`
	GroupOverride             bool                      `json:"group_override,omitempty"`
	GroupWeight               int                       `json:"group_weight,omitempty"`
	PreventRecursion          bool                      `json:"prevent_recursion,omitempty"`
	DelayUntilRecursion       any                       `json:"delay_until_recursion,omitempty"`
	ScanDepth                 int                       `json:"scan_depth,omitempty"`
	MatchWholeWords           *bool                     `json:"match_whole_words,omitempty"`
	UseGroupScoring           bool                      `json:"use_group_scoring,omitempty"`
	CaseSensitive             *bool                     `json:"case_sensitive,omitempty"`
	AutomationId              string                    `json:"automation_id,omitempty"`
	Role                      Role                      `json:"role,omitempty"`
	Vectorized                bool                      `json:"vectorized,omitempty"`
	Sticky                    int                       `json:"sticky,omitempty"`
	Cooldown                  int                       `json:"cooldown,omitempty"`
	Delay                     int                       `json:"delay,omitempty"`
	MatchPersonaDescription   bool                      `json:"match_persona_description,omitempty"`
	MatchCharacterDescription bool                      `json:"match_character_description,omitempty"`
	MatchCharacterPersonality bool                      `json:"match_character_personality,omitempty"`
	MatchCharacterDepthPrompt bool                      `json:"match_character_depth_prompt,omitempty"`
	MatchScenario             bool                      `json:"match_scenario,omitempty"`
	MatchCreatorNotes         bool                      `json:"match_creator_notes,omitempty"`
	Triggers                  []interface{}             `json:"triggers,omitempty"`
}

// LorebookInsertionPosition 定义设定集条目的插入位置
type LorebookInsertionPosition int

const (
	/*
		Before Char Defs:
		World Info entry is inserted before the character's description and scenario.
		Has a moderate impact on the conversation.
	*/
	LorebookInsertionBeforeCharDefs LorebookInsertionPosition = iota
	/*
		After Char Defs:
		World Info entry is inserted after the character's description and scenario.
		Has a greater impact on the conversation.
	*/
	LorebookInsertionAfterCharDefs
	/*
		Top of AN:
		World Info entry is inserted at the top of Author's Note content.
		Has a variable impact depending on the Author's Note position.
	*/
	LorebookInsertionTopOfAuthorsNote
	/*
		Bottom of AN:
		World Info entry is inserted at the bottom of Author's Note content.
		Has a variable impact depending on the Author's Note position.
	*/
	LorebookInsertionBottomOfAuthorsNote
	/*
		@D:
		World Info entry is inserted at a specific depth in the chat.
		Depth 0 being the bottom of the prompt.
	*/
	LorebookInsertionAtDepth
	/*
		Before Example Messages:
		The World Info entry is parsed as an example dialogue block and inserted before the examples provided by the character card.
	*/
	LorebookInsertionBeforeExampleMessages
	/*
		After Example Messages:
		The World Info entry is parsed as an example dialogue block and inserted after the examples provided by the character card.
	*/
	LorebookInsertionAfterExampleMessages
)
