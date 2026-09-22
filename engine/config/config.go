package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Backend                                 string
	DataDir                                 string
	GameDataDir                             string
	SchemaDir                               string
	UpdatesEnableDatabases                  uint32
	UpdatesAutoSetup                        bool
	UpdatesRedundancy                       bool
	UpdatesArchivedRedundancy               bool
	UpdatesAllowRehash                      bool
	UpdatesCleanDeadReferencesMaxCount      int
	AuthDatabaseFile                        string
	WrongPassMaxCount                       uint32
	WrongPassBanTime                        uint32
	WrongPassBanType                        bool
	WrongPassLogging                        bool
	StrictVersionCheck                      bool
	CharactersDatabaseFile                  string
	WorldDatabaseFile                       string
	LoginDatabaseInfo                       string
	WorldDatabaseInfo                       string
	CharacterDatabaseInfo                   string
	ServerLoginInfo                         bool
	RealmAddress                            string
	RealmServerPort                         int
	WorldServerPort                         int
	RealmID                                 uint32
	LogsDir                                 string
	ProtocolTracePath                       string
	Motd                                    string
	ClientCacheVersion                      uint32
	LuaEnabled                              bool
	LuaScriptPath                           string
	CharacterCreatingDisabled               uint32
	CharacterCreatingDisabledRaceMask       uint32
	CharacterCreatingDisabledClassMask      uint32
	CharactersPerAccount                    uint32
	CharactersPerRealm                      uint32
	DeathKnightsPerRealm                    uint32
	CharacterCreatingMinLevelForDeathKnight uint32
	Expansion                               uint32
	MaxPlayerLevel                          uint32
	MaxPrimaryTradeSkill                    uint32
	StartPlayerMoney                        uint32
	StartPlayerLevel                        uint32
	StartDeathKnightPlayerLevel             uint32
	GameType                                uint32
	AllowTwoSideInteractionGroup            bool
	StartHonorPoints                        uint32
	StartArenaPoints                        uint32
	AllFlightPaths                          bool
	AlwaysMaxSkillForLevel                  bool
	RestOfflineInTavernOrCityRate           float64
	RestOfflineInWildernessRate             float64
	DisableFatigue                          int
	VisibilityDistanceContinents            float64
	WeatherEnabled                          bool
	WeatherChangeInterval                   uint32
	SoloLFGEnable                           bool
	SoloLFGAnnounce                         bool
	GMLoginState                            int
	GMVisibleState                          int
	ChatFloodMessageCount                   uint32
	ChatFloodMessageDelay                   uint32
	ChatFloodMuteTime                       uint32
	AddonChannel                            bool
	ChatChannelLevelReq                     uint32
	ChatWhisperLevelReq                     uint32
	ChatEmoteLevelReq                       uint32
	ChatSayLevelReq                         uint32
	ChatYellLevelReq                        uint32
	MaxOverSpeedPings                       uint32
	MinPetitionSigns                        uint32
	DeathCorpseReclaimDelayPvE              bool
	DeathCorpseReclaimDelayPvP              bool
	DeathBonesWorld                         bool
	DeathBonesBattleground                  bool
	PlayerStartAllSpells                    bool
	PlayerStartAllReputation                bool
	PlayerStartMapsExplored                 bool
	PlayerStartString                       string
	ArenaSeasonID                           uint32
	ArenaSeasonInProgress                   bool
	WardenEnabled                           bool
	WardenNumInjectionChecks                uint32
	WardenNumLuaSandboxChecks               uint32
	WardenNumClientModChecks                uint32
	WardenClientResponseDelay               uint32
	WardenClientCheckHoldOff                uint32
	WardenClientCheckFailAction             uint32
	WardenBanDuration                       uint32
	NPCBots                                 NPCBotConfig
	UnrecognizedKeys                        []string
}

type NPCBotConfig struct {
	Enable                   bool
	MaxBots                  uint32
	MaxBotsPerClass          uint32
	BaseFollowDistance       uint32
	XPReduction              uint32
	HealTargetIconsMask      uint32
	TankTargetIconMask       uint32
	DPSTargetIconMask        uint32
	DamagePhysicalMultiplier float64
	DamageSpellMultiplier    float64
	HealingMultiplier        float64
	EnableDungeon            bool
	EnableRaid               bool
	EnableBG                 bool
	EnableArena              bool
	EnableDungeonFinder      bool
	LimitDungeon             bool
	LimitRaid                bool
	Cost                     uint64
	UpdateDelayBase          uint32
	OwnershipExpireTime      uint32
	PvP                      bool
	MovementInterruptFood    bool
	EquipmentDisplayEnable   bool
	ShowCloak                bool
	ShowHelm                 bool
	BlademasterEnable        bool
	ObsidianDestroyerEnable  bool
	ArchmageEnable           bool
	DreadlordEnable          bool
	SpellBreakerEnable       bool
	DarkRangerEnable         bool
	StatsLimitsEnable        bool
	StatLimitDodge           float64
	StatLimitParry           float64
	StatLimitBlock           float64
	StatLimitCrit            float64
}

func defaultConfig() (c Config) {
	defer func() {
		c.DeathBonesWorld = true
		c.DeathBonesBattleground = true
		c.WeatherEnabled = true
		c.WeatherChangeInterval = 600000
		c.AddonChannel = true
		c.MaxPrimaryTradeSkill = 2
		c.NPCBots.DamagePhysicalMultiplier = 1
		c.NPCBots.DamageSpellMultiplier = 1
	}()
	return Config{Backend: "sqlite", DataDir: ".", GameDataDir: "data", SchemaDir: "sql", UpdatesEnableDatabases: 7, UpdatesAutoSetup: true, UpdatesRedundancy: true, UpdatesArchivedRedundancy: false, UpdatesAllowRehash: true, UpdatesCleanDeadReferencesMaxCount: 3, AuthDatabaseFile: "auth.db", WrongPassBanTime: 600, CharactersDatabaseFile: "characters.db", WorldDatabaseFile: "world.db", RealmServerPort: 3724, WorldServerPort: 8085, RealmID: 1, LogsDir: "logs", Motd: "Welcome to a Trinity Core server.", ClientCacheVersion: 0, LuaEnabled: true, LuaScriptPath: "lua_scripts", CharacterCreatingDisabled: 0, CharacterCreatingDisabledRaceMask: 0, CharacterCreatingDisabledClassMask: 0, CharactersPerAccount: 50, CharactersPerRealm: 10, DeathKnightsPerRealm: 1, CharacterCreatingMinLevelForDeathKnight: 55, Expansion: 2, MaxPlayerLevel: 80, StartPlayerLevel: 1, StartPlayerMoney: 10000, GameType: 0, AllFlightPaths: false, AlwaysMaxSkillForLevel: true, DisableFatigue: 4, VisibilityDistanceContinents: 100, SoloLFGEnable: true, SoloLFGAnnounce: true, GMLoginState: 2, GMVisibleState: 2, ChatFloodMessageCount: 10, ChatFloodMessageDelay: 1, ChatFloodMuteTime: 10, ChatChannelLevelReq: 1, ChatWhisperLevelReq: 1, ChatEmoteLevelReq: 1, ChatSayLevelReq: 1, ChatYellLevelReq: 1, MaxOverSpeedPings: 2, MinPetitionSigns: 9, DeathCorpseReclaimDelayPvE: true, DeathCorpseReclaimDelayPvP: true, DeathBonesWorld: true, DeathBonesBattleground: true, PlayerStartAllSpells: false, PlayerStartAllReputation: false, PlayerStartMapsExplored: false, ArenaSeasonID: 8, ArenaSeasonInProgress: true, WardenEnabled: false, WardenNumInjectionChecks: 9, WardenNumLuaSandboxChecks: 1, WardenNumClientModChecks: 1, WardenClientResponseDelay: 600, WardenClientCheckHoldOff: 30, WardenClientCheckFailAction: 0, WardenBanDuration: 86400, NPCBots: NPCBotConfig{Enable: true, MaxBots: 9, MaxBotsPerClass: 0, BaseFollowDistance: 25, XPReduction: 0, HealTargetIconsMask: 0, TankTargetIconMask: 0, DPSTargetIconMask: 0, HealingMultiplier: 1, EnableDungeon: true, EnableRaid: true, EnableBG: true, EnableArena: true, EnableDungeonFinder: true, LimitDungeon: true, LimitRaid: true, Cost: 1000000, UpdateDelayBase: 0, OwnershipExpireTime: 0, PvP: true, EquipmentDisplayEnable: true, ShowCloak: true, ShowHelm: true, BlademasterEnable: false, ObsidianDestroyerEnable: false, ArchmageEnable: false, DreadlordEnable: false, SpellBreakerEnable: false, DarkRangerEnable: false, StatsLimitsEnable: false, StatLimitDodge: 95, StatLimitParry: 95, StatLimitBlock: 95, StatLimitCrit: 95}}
}

func Default() Config {
	c := defaultConfig()
	c.RestOfflineInTavernOrCityRate = 1
	c.RestOfflineInWildernessRate = 1
	return c
}

func Load(path string) (Config, error) {
	c := Default()
	if path == "" {
		return c, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	line := 0
	for s.Scan() {
		line++
		key, value, ok := split(s.Text())
		if !ok {
			continue
		}
		if err := c.set(key, value); err != nil {
			return c, fmt.Errorf("%s:%d: %w", path, line, err)
		}
	}
	if err := s.Err(); err != nil {
		return c, err
	}
	return c, nil
}

func (c *Config) ApplyEnv() {
	values := map[string]string{"MORENOCORE_BACKEND": "Database.Backend", "MORENOCORE_DATA_DIR": "DataDir", "MORENOCORE_SCHEMA_DIR": "SchemaDir", "MORENOCORE_AUTH_DB": "AuthDatabaseFile", "MORENOCORE_CHARACTERS_DB": "CharactersDatabaseFile", "MORENOCORE_WORLD_DB": "WorldDatabaseFile", "MORENOCORE_LOGIN_DATABASE": "LoginDatabaseInfo", "MORENOCORE_WORLD_DATABASE": "WorldDatabaseInfo", "MORENOCORE_CHARACTER_DATABASE": "CharacterDatabaseInfo", "MORENOCORE_REALM_PORT": "RealmServerPort", "MORENOCORE_WORLD_PORT": "WorldServerPort", "MORENOCORE_REALM_ID": "RealmID", "MORENOCORE_LOGS_DIR": "LogsDir", "MORENOCORE_PROTOCOL_TRACE": "ProtocolTracePath", "MORENOCORE_LUA_ENABLED": "Eluna.Enabled", "MORENOCORE_LUA_PATH": "Eluna.ScriptPath", "MORENOCORE_CHARACTER_CREATING_DISABLED": "CharacterCreating.Disabled", "MORENOCORE_CHARACTER_CREATING_DISABLED_RACE_MASK": "CharacterCreating.Disabled.RaceMask", "MORENOCORE_CHARACTER_CREATING_DISABLED_CLASS_MASK": "CharacterCreating.Disabled.ClassMask", "MORENOCORE_CHARACTERS_PER_ACCOUNT": "CharactersPerAccount", "MORENOCORE_CHARACTERS_PER_REALM": "CharactersPerRealm", "MORENOCORE_DEATH_KNIGHTS_PER_REALM": "DeathKnightsPerRealm", "MORENOCORE_MIN_LEVEL_DEATH_KNIGHT": "CharacterCreating.MinLevelForDeathKnight", "MORENOCORE_EXPANSION": "Expansion", "MORENOCORE_START_PLAYER_MONEY": "StartPlayerMoney", "MORENOCORE_START_PLAYER_LEVEL": "StartPlayerLevel", "MORENOCORE_START_DEATH_KNIGHT_PLAYER_LEVEL": "StartDeathKnightPlayerLevel", "MORENOCORE_START_HONOR_POINTS": "StartHonorPoints", "MORENOCORE_START_ARENA_POINTS": "StartArenaPoints", "MORENOCORE_ALWAYS_MAX_SKILL_FOR_LEVEL": "AlwaysMaxSkillForLevel", "MORENOCORE_DISABLE_FATIGUE": "DisableFatigue", "MORENOCORE_VISIBILITY_DISTANCE_CONTINENTS": "Visibility.Distance.Continents", "MORENOCORE_SOLO_LFG_ENABLE": "SoloLFG.Enable", "MORENOCORE_SOLO_LFG_ANNOUNCE": "SoloLFG.Announce", "MORENOCORE_GM_LOGIN_STATE": "GM.LoginState", "MORENOCORE_GM_VISIBLE_STATE": "GM.VisibleState", "MORENOCORE_CHAT_FLOOD_MESSAGE_COUNT": "ChatFlood.MessageCount", "MORENOCORE_CHAT_FLOOD_MESSAGE_DELAY": "ChatFlood.MessageDelay", "MORENOCORE_CHAT_FLOOD_MUTE_TIME": "ChatFlood.MuteTime", "MORENOCORE_CHAT_CHANNEL_LEVEL_REQ": "ChatLevelReq.Channel", "MORENOCORE_CHAT_WHISPER_LEVEL_REQ": "ChatLevelReq.Whisper", "MORENOCORE_CHAT_EMOTE_LEVEL_REQ": "ChatLevelReq.Emote", "MORENOCORE_CHAT_SAY_LEVEL_REQ": "ChatLevelReq.Say", "MORENOCORE_CHAT_YELL_LEVEL_REQ": "ChatLevelReq.Yell", "MORENOCORE_MAX_OVERSPEED_PINGS": "MaxOverspeedPings", "MORENOCORE_DEATH_CORPSE_RECLAIM_DELAY_PVE": "Death.CorpseReclaimDelay.PvE", "MORENOCORE_DEATH_CORPSE_RECLAIM_DELAY_PVP": "Death.CorpseReclaimDelay.PvP", "MORENOCORE_PLAYERSTART_ALL_SPELLS": "PlayerStart.AllSpells", "MORENOCORE_PLAYERSTART_ALL_REPUTATION": "PlayerStart.AllReputation", "MORENOCORE_PLAYERSTART_MAPS_EXPLORED": "PlayerStart.MapsExplored", "MORENOCORE_PLAYERSTART_STRING": "PlayerStart.String", "MORENOCORE_ARENA_SEASON_ID": "Arena.ArenaSeason.ID", "MORENOCORE_ARENA_SEASON_IN_PROGRESS": "Arena.ArenaSeason.InProgress", "MORENOCORE_WARDEN_ENABLED": "Warden.Enabled", "MORENOCORE_WARDEN_NUM_INJECTION_CHECKS": "Warden.NumInjectionChecks", "MORENOCORE_WARDEN_NUM_LUA_SANDBOX_CHECKS": "Warden.NumLuaSandboxChecks", "MORENOCORE_WARDEN_NUM_CLIENT_MOD_CHECKS": "Warden.NumClientModChecks", "MORENOCORE_WARDEN_CLIENT_RESPONSE_DELAY": "Warden.ClientResponseDelay", "MORENOCORE_WARDEN_CLIENT_CHECK_HOLD_OFF": "Warden.ClientCheckHoldOff", "MORENOCORE_WARDEN_CLIENT_CHECK_FAIL_ACTION": "Warden.ClientCheckFailAction", "MORENOCORE_WARDEN_BAN_DURATION": "Warden.BanDuration", "MORENOCORE_NPCBOT_ENABLE": "NpcBot.Enable", "MORENOCORE_NPCBOT_MAX_BOTS": "NpcBot.MaxBots", "MORENOCORE_NPCBOT_MAX_BOTS_PER_CLASS": "NpcBot.MaxBotsPerClass", "MORENOCORE_NPCBOT_BASE_FOLLOW_DISTANCE": "NpcBot.BaseFollowDistance", "MORENOCORE_NPCBOT_XP_REDUCTION": "NpcBot.XpReduction", "MORENOCORE_NPCBOT_HEAL_TARGET_ICONS_MASK": "NpcBot.HealTargetIconsMask", "MORENOCORE_NPCBOT_TANK_TARGET_ICON_MASK": "NpcBot.TankTargetIconMask", "MORENOCORE_NPCBOT_DPS_TARGET_ICON_MASK": "NpcBot.DPSTargetIconMask", "MORENOCORE_NPCBOT_DAMAGE_PHYSICAL": "NpcBot.Mult.Damage.Physical", "MORENOCORE_NPCBOT_DAMAGE_SPELL": "NpcBot.Mult.Damage.Spell", "MORENOCORE_NPCBOT_HEALING": "NpcBot.Mult.Healing", "MORENOCORE_NPCBOT_ENABLE_DUNGEON": "NpcBot.Enable.Dungeon", "MORENOCORE_NPCBOT_ENABLE_RAID": "NpcBot.Enable.Raid", "MORENOCORE_NPCBOT_ENABLE_BG": "NpcBot.Enable.BG", "MORENOCORE_NPCBOT_ENABLE_ARENA": "NpcBot.Enable.Arena", "MORENOCORE_NPCBOT_ENABLE_DUNGEON_FINDER": "NpcBot.Enable.DungeonFinder", "MORENOCORE_NPCBOT_LIMIT_DUNGEON": "NpcBot.Limit.Dungeon", "MORENOCORE_NPCBOT_LIMIT_RAID": "NpcBot.Limit.Raid", "MORENOCORE_NPCBOT_COST": "NpcBot.Cost", "MORENOCORE_NPCBOT_UPDATE_DELAY_BASE": "NpcBot.UpdateDelay.Base", "MORENOCORE_NPCBOT_OWNERSHIP_EXPIRE_TIME": "NpcBot.OwnershipExpireTime", "MORENOCORE_NPCBOT_PVP": "NpcBot.PvP", "MORENOCORE_NPCBOT_INTERRUPT_FOOD": "NpcBot.Movements.InterruptFood", "MORENOCORE_NPCBOT_EQUIPMENT_DISPLAY": "NpcBot.EquipmentDisplay.Enable", "MORENOCORE_NPCBOT_SHOW_CLOAK": "NpcBot.EquipmentDisplay.ShowCloak", "MORENOCORE_NPCBOT_SHOW_HELM": "NpcBot.EquipmentDisplay.ShowHelm", "MORENOCORE_NPCBOT_BLADEMASTER": "NpcBot.NewClasses.Blademaster.Enable", "MORENOCORE_NPCBOT_OBSIDIAN_DESTROYER": "NpcBot.NewClasses.ObsidianDestroyer.Enable", "MORENOCORE_NPCBOT_ARCHMAGE": "NpcBot.NewClasses.Archmage.Enable", "MORENOCORE_NPCBOT_DREADLORD": "NpcBot.NewClasses.Dreadlord.Enable", "MORENOCORE_NPCBOT_SPELL_BREAKER": "NpcBot.NewClasses.SpellBreaker.Enable", "MORENOCORE_NPCBOT_DARK_RANGER": "NpcBot.NewClasses.DarkRanger.Enable", "MORENOCORE_NPCBOT_STATS_LIMITS": "NpcBot.Stats.Limits.Enable", "MORENOCORE_NPCBOT_STAT_DODGE": "NpcBot.Stats.Limits.Dodge", "MORENOCORE_NPCBOT_STAT_PARRY": "NpcBot.Stats.Limits.Parry", "MORENOCORE_NPCBOT_STAT_BLOCK": "NpcBot.Stats.Limits.Block", "MORENOCORE_NPCBOT_STAT_CRIT": "NpcBot.Stats.Limits.Crit"}
	values["MORENOCORE_GAME_TYPE"] = "GameType"
	values["MORENOCORE_RATE_REST_OFFLINE_IN_TAVERN_OR_CITY"] = "Rate.Rest.Offline.InTavernOrCity"
	values["MORENOCORE_RATE_REST_OFFLINE_IN_WILDERNESS"] = "Rate.Rest.Offline.InWilderness"
	values["MORENOCORE_ADDON_CHANNEL"] = "AddonChannel"
	values["MORENOCORE_SERVER_LOGIN_INFO"] = "Server.LoginInfo"
	values["MORENOCORE_MAX_PLAYER_LEVEL"] = "MaxPlayerLevel"
	values["MORENOCORE_MAX_PRIMARY_TRADE_SKILL"] = "MaxPrimaryTradeSkill"
	values["MORENOCORE_ALL_FLIGHT_PATHS"] = "AllFlightPaths"
	values["MORENOCORE_WRONGPASS_MAX_COUNT"] = "WrongPass.MaxCount"
	values["MORENOCORE_CLIENT_CACHE_VERSION"] = "ClientCacheVersion"
	values["MORENOCORE_WRONGPASS_BAN_TIME"] = "WrongPass.BanTime"
	values["MORENOCORE_WRONGPASS_BAN_TYPE"] = "WrongPass.BanType"
	values["MORENOCORE_WRONGPASS_LOGGING"] = "WrongPass.Logging"
	values["MORENOCORE_STRICT_VERSION_CHECK"] = "StrictVersionCheck"
	values["MORENOCORE_UPDATES_ENABLE_DATABASES"] = "Updates.EnableDatabases"
	values["MORENOCORE_UPDATES_AUTO_SETUP"] = "Updates.AutoSetup"
	values["MORENOCORE_UPDATES_REDUNDANCY"] = "Updates.Redundancy"
	values["MORENOCORE_UPDATES_ARCHIVED_REDUNDANCY"] = "Updates.ArchivedRedundancy"
	values["MORENOCORE_UPDATES_ALLOW_REHASH"] = "Updates.AllowRehash"
	values["MORENOCORE_UPDATES_CLEAN_DEAD_REF_MAX_COUNT"] = "Updates.CleanDeadRefMaxCount"
	for env, key := range values {
		if value, ok := os.LookupEnv(env); ok {
			_ = c.set(key, value)
		}
	}
	if value, ok := os.LookupEnv("MORENOCORE_DEATH_BONES_WORLD"); ok {
		_ = c.set("Death.Bones.World", value)
	}
	if value, ok := os.LookupEnv("MORENOCORE_DEATH_BONES_BATTLEGROUND"); ok {
		_ = c.set("Death.Bones.BattlegroundOrArena", value)
	}
	if value, ok := os.LookupEnv("MORENOCORE_REALM_ADDRESS"); ok {
		_ = c.set("RealmAddress", value)
	}
	if value, ok := os.LookupEnv("MORENOCORE_MOTD"); ok {
		_ = c.set("Motd", value)
	}
	if value, ok := os.LookupEnv("MORENOCORE_GAME_DATA_DIR"); ok {
		c.GameDataDir = value
	}
	if value, ok := os.LookupEnv("MORENOCORE_WEATHER_ENABLED"); ok {
		_ = c.set("Weather.Enabled", value)
	}
	if value, ok := os.LookupEnv("MORENOCORE_WEATHER_CHANGE_INTERVAL"); ok {
		_ = c.set("Weather.ChangeInterval", value)
	}
}

func (c *Config) Set(key, value string) error { return c.set(key, value) }

func (c *Config) ApplyWorkDir(workDir string) {
	if workDir == "" {
		return
	}
	cleanWork := filepath.Clean(workDir)
	if !filepath.IsAbs(c.DataDir) {
		cleanData := filepath.Clean(c.DataDir)
		if cleanData == "." || cleanData == "" || strings.EqualFold(cleanData, cleanWork) {
			c.DataDir = cleanWork
		} else {
			if _, err := os.Stat(c.DataDir); err != nil {
				c.DataDir = filepath.Clean(filepath.Join(cleanWork, c.DataDir))
			}
		}
	}
	if !filepath.IsAbs(c.LogsDir) {
		cleanLogs := filepath.Clean(c.LogsDir)
		if cleanLogs == "." || cleanLogs == "" || strings.EqualFold(cleanLogs, cleanWork) {
			c.LogsDir = filepath.Join(cleanWork, "logs")
		} else {
			c.LogsDir = filepath.Clean(filepath.Join(cleanWork, c.LogsDir))
		}
	}
	c.GameDataDir = resolveGameDataPath(
		filepath.Join(cleanWork, c.GameDataDir),
		filepath.Join(cleanWork, "data"),
		c.GameDataDir,
		filepath.Join("bin", c.GameDataDir),
		filepath.Join("bin", "data"),
		"data",
		filepath.Join("..", c.GameDataDir),
	)
	c.SchemaDir = resolvePath(
		filepath.Join(cleanWork, c.SchemaDir),
		filepath.Join(cleanWork, "sql"),
		c.SchemaDir,
		filepath.Join("bin", c.SchemaDir),
		filepath.Join("bin", "sql"),
		"sql",
		filepath.Join("..", c.SchemaDir),
	)
	c.LuaScriptPath = resolvePath(
		filepath.Join(cleanWork, c.LuaScriptPath),
		filepath.Join(cleanWork, "lua_scripts"),
		c.LuaScriptPath,
		filepath.Join("bin", c.LuaScriptPath),
		filepath.Join("bin", "lua_scripts"),
		"lua_scripts",
		filepath.Join("..", c.LuaScriptPath),
	)
}

func (c *Config) ResolvePaths() {
	c.DataDir = resolvePath(c.DataDir, "bin", filepath.Join("..", "bin"), filepath.Join("..", "..", "bin"))
	c.GameDataDir = resolveGameDataPath(c.GameDataDir, filepath.Join(c.DataDir, "data"), c.DataDir, filepath.Join("bin", c.GameDataDir), filepath.Join("bin", "data"), filepath.Join("..", c.GameDataDir))
	c.SchemaDir = resolvePath(c.SchemaDir, filepath.Join("bin", c.SchemaDir), filepath.Join("bin", "sql"), filepath.Join("..", c.SchemaDir))
	c.LuaScriptPath = resolvePath(c.LuaScriptPath, filepath.Join(c.DataDir, "lua_scripts"), filepath.Join("bin", c.LuaScriptPath), filepath.Join("bin", "lua_scripts"), filepath.Join("..", c.LuaScriptPath))
	if c.ProtocolTracePath != "" && !filepath.IsAbs(c.ProtocolTracePath) {
		c.ProtocolTracePath = filepath.Join(c.DataDir, c.ProtocolTracePath)
	}
	c.AuthDatabaseFile = c.DatabasePath(c.AuthDatabaseFile)
	c.CharactersDatabaseFile = c.DatabasePath(c.CharactersDatabaseFile)
	c.WorldDatabaseFile = c.DatabasePath(c.WorldDatabaseFile)
}

func resolvePath(path string, alternatives ...string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	for _, candidate := range append([]string{path}, alternatives...) {
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Clean(candidate)
		}
	}
	return filepath.Clean(path)
}

func resolveGameDataPath(path string, alternatives ...string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	for _, candidate := range append([]string{path}, alternatives...) {
		if candidate == "" {
			continue
		}
		complete := true
		for _, name := range []string{"ChrRaces.dbc", "ChrClasses.dbc", "Spell.dbc", "Map.dbc"} {
			info, err := os.Stat(filepath.Join(candidate, "dbc", name))
			if err != nil || info.IsDir() {
				complete = false
				break
			}
		}
		if complete {
			return candidate
		}
	}
	return resolvePath(path, alternatives...)
}

func (c Config) DatabasePath(name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	candidates := []string{
		filepath.Join(c.DataDir, name),
		name,
		filepath.Join("bin", name),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return filepath.Clean(candidate)
		}
	}
	return filepath.Clean(filepath.Join(c.DataDir, name))
}

func split(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
		return "", "", false
	}
	i := strings.IndexByte(line, '=')
	if i < 0 {
		return "", "", false
	}
	key := strings.TrimSpace(line[:i])
	value := strings.TrimSpace(line[i+1:])
	if n := strings.Index(value, " #"); n >= 0 {
		value = strings.TrimSpace(value[:n])
	}
	if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
		value = value[1 : len(value)-1]
	}
	return key, value, true
}

func (c *Config) set(key, value string) error {
	switch key {
	case "Database.Backend":
		c.Backend = strings.ToLower(value)
	case "DataDir":
		c.DataDir = value
	case "GameDataDir":
		c.GameDataDir = value
	case "SchemaDir":
		c.SchemaDir = value
	case "Updates.EnableDatabases":
		return setUint32(&c.UpdatesEnableDatabases, key, value)
	case "Updates.AutoSetup":
		return setBool(&c.UpdatesAutoSetup, key, value)
	case "Updates.Redundancy":
		return setBool(&c.UpdatesRedundancy, key, value)
	case "Updates.ArchivedRedundancy":
		return setBool(&c.UpdatesArchivedRedundancy, key, value)
	case "Updates.AllowRehash":
		return setBool(&c.UpdatesAllowRehash, key, value)
	case "Updates.CleanDeadRefMaxCount":
		return setInt(&c.UpdatesCleanDeadReferencesMaxCount, key, value)
	case "AuthDatabaseFile":
		c.AuthDatabaseFile = value
	case "WrongPass.MaxCount":
		return setUint32(&c.WrongPassMaxCount, key, value)
	case "WrongPass.BanTime":
		return setUint32(&c.WrongPassBanTime, key, value)
	case "WrongPass.BanType":
		return setBool(&c.WrongPassBanType, key, value)
	case "WrongPass.Logging":
		return setBool(&c.WrongPassLogging, key, value)
	case "StrictVersionCheck":
		return setBool(&c.StrictVersionCheck, key, value)
	case "CharactersDatabaseFile":
		c.CharactersDatabaseFile = value
	case "WorldDatabaseFile":
		c.WorldDatabaseFile = value
	case "LoginDatabaseInfo":
		c.LoginDatabaseInfo = value
	case "WorldDatabaseInfo":
		c.WorldDatabaseInfo = value
	case "CharacterDatabaseInfo":
		c.CharacterDatabaseInfo = value
	case "Server.LoginInfo":
		return setBool(&c.ServerLoginInfo, key, value)
	case "RealmAddress":
		c.RealmAddress = value
	case "RealmServerPort":
		return setInt(&c.RealmServerPort, key, value)
	case "WorldServerPort":
		return setInt(&c.WorldServerPort, key, value)
	case "RealmID":
		return setUint32(&c.RealmID, key, value)
	case "LogsDir":
		c.LogsDir = value
	case "ProtocolTracePath":
		c.ProtocolTracePath = value
	case "Motd":
		c.Motd = value
	case "ClientCacheVersion":
		return setUint32(&c.ClientCacheVersion, key, value)
	case "Eluna.Enabled":
		return setBool(&c.LuaEnabled, key, value)
	case "Eluna.ScriptPath":
		c.LuaScriptPath = value
	case "CharacterCreating.Disabled":
		return setUint32(&c.CharacterCreatingDisabled, key, value)
	case "CharacterCreating.Disabled.RaceMask":
		return setUint32(&c.CharacterCreatingDisabledRaceMask, key, value)
	case "CharacterCreating.Disabled.ClassMask":
		return setUint32(&c.CharacterCreatingDisabledClassMask, key, value)
	case "CharactersPerAccount":
		return setUint32(&c.CharactersPerAccount, key, value)
	case "CharactersPerRealm":
		return setUint32(&c.CharactersPerRealm, key, value)
	case "DeathKnightsPerRealm":
		return setUint32(&c.DeathKnightsPerRealm, key, value)
	case "CharacterCreating.MinLevelForDeathKnight":
		return setUint32(&c.CharacterCreatingMinLevelForDeathKnight, key, value)
	case "Expansion":
		return setUint32(&c.Expansion, key, value)
	case "MaxPlayerLevel":
		return setUint32(&c.MaxPlayerLevel, key, value)
	case "MaxPrimaryTradeSkill":
		return setUint32(&c.MaxPrimaryTradeSkill, key, value)
	case "StartPlayerMoney":
		return setUint32(&c.StartPlayerMoney, key, value)
	case "StartPlayerLevel":
		return setUint32(&c.StartPlayerLevel, key, value)
	case "StartDeathKnightPlayerLevel":
		return setUint32(&c.StartDeathKnightPlayerLevel, key, value)
	case "GameType":
		return setUint32(&c.GameType, key, value)
	case "AllowTwoSide.Interaction.Group":
		return setBool(&c.AllowTwoSideInteractionGroup, key, value)
	case "AllFlightPaths":
		return setBool(&c.AllFlightPaths, key, value)
	case "StartHonorPoints":
		return setUint32(&c.StartHonorPoints, key, value)
	case "StartArenaPoints":
		return setUint32(&c.StartArenaPoints, key, value)
	case "AlwaysMaxSkillForLevel":
		return setBool(&c.AlwaysMaxSkillForLevel, key, value)
	case "DisableFatigue":
		return setInt(&c.DisableFatigue, key, value)
	case "Rate.Rest.Offline.InTavernOrCity":
		return setFloat64(&c.RestOfflineInTavernOrCityRate, key, value)
	case "Rate.Rest.Offline.InWilderness":
		return setFloat64(&c.RestOfflineInWildernessRate, key, value)
	case "Visibility.Distance.Continents":
		return setFloat64(&c.VisibilityDistanceContinents, key, value)
	case "Weather.Enabled":
		return setBool(&c.WeatherEnabled, key, value)
	case "Weather.ChangeInterval":
		return setUint32(&c.WeatherChangeInterval, key, value)
	case "ActivateWeather":
		return setBool(&c.WeatherEnabled, key, value)
	case "ChangeWeatherInterval":
		return setUint32(&c.WeatherChangeInterval, key, value)
	case "GM.LoginState":
		return setInt(&c.GMLoginState, key, value)
	case "GM.VisibleState":
		return setInt(&c.GMVisibleState, key, value)
	case "ChatFlood.MessageCount":
		return setUint32(&c.ChatFloodMessageCount, key, value)
	case "ChatFlood.MessageDelay":
		return setUint32(&c.ChatFloodMessageDelay, key, value)
	case "ChatFlood.MuteTime":
		return setUint32(&c.ChatFloodMuteTime, key, value)
	case "AddonChannel":
		return setBool(&c.AddonChannel, key, value)
	case "ChatLevelReq.Channel":
		return setUint32(&c.ChatChannelLevelReq, key, value)
	case "ChatLevelReq.Whisper":
		return setUint32(&c.ChatWhisperLevelReq, key, value)
	case "ChatLevelReq.Emote":
		return setUint32(&c.ChatEmoteLevelReq, key, value)
	case "ChatLevelReq.Say":
		return setUint32(&c.ChatSayLevelReq, key, value)
	case "ChatLevelReq.Yell":
		return setUint32(&c.ChatYellLevelReq, key, value)
	case "MaxOverspeedPings":
		var pingLimit uint32
		if err := setUint32(&pingLimit, key, value); err != nil {
			return err
		}
		// Reference: World.cpp SetConfig — non-zero values below 2 are forced to 2.
		if pingLimit != 0 && pingLimit < 2 {
			pingLimit = 2
		}
		c.MaxOverSpeedPings = pingLimit
		return nil
	case "MinPetitionSigns":
		return setUint32(&c.MinPetitionSigns, key, value)
	case "Death.CorpseReclaimDelay.PvE":
		return setBool(&c.DeathCorpseReclaimDelayPvE, key, value)
	case "Death.CorpseReclaimDelay.PvP":
		return setBool(&c.DeathCorpseReclaimDelayPvP, key, value)
	case "Death.Bones.World":
		return setBool(&c.DeathBonesWorld, key, value)
	case "Death.Bones.BattlegroundOrArena":
		return setBool(&c.DeathBonesBattleground, key, value)
	case "PlayerStart.AllSpells":
		return setBool(&c.PlayerStartAllSpells, key, value)
	case "PlayerStart.AllReputation":
		return setBool(&c.PlayerStartAllReputation, key, value)
	case "PlayerStart.MapsExplored":
		return setBool(&c.PlayerStartMapsExplored, key, value)
	case "PlayerStart.String":
		c.PlayerStartString = value
	case "Arena.ArenaSeason.ID":
		return setUint32(&c.ArenaSeasonID, key, value)
	case "Arena.ArenaSeason.InProgress":
		return setBool(&c.ArenaSeasonInProgress, key, value)
	case "Warden.Enabled":
		return setBool(&c.WardenEnabled, key, value)
	case "Warden.NumInjectionChecks":
		return setUint32(&c.WardenNumInjectionChecks, key, value)
	case "Warden.NumLuaSandboxChecks":
		return setUint32(&c.WardenNumLuaSandboxChecks, key, value)
	case "Warden.NumClientModChecks":
		return setUint32(&c.WardenNumClientModChecks, key, value)
	case "Warden.ClientResponseDelay":
		return setUint32(&c.WardenClientResponseDelay, key, value)
	case "Warden.ClientCheckHoldOff":
		return setUint32(&c.WardenClientCheckHoldOff, key, value)
	case "Warden.ClientCheckFailAction":
		return setUint32(&c.WardenClientCheckFailAction, key, value)
	case "Warden.BanDuration":
		return setUint32(&c.WardenBanDuration, key, value)
	case "SoloLFG.Enable":
		return setBool(&c.SoloLFGEnable, key, value)
	case "SoloLFG.Announce":
		return setBool(&c.SoloLFGAnnounce, key, value)
	case "NpcBot.Enable":
		return setBool(&c.NPCBots.Enable, key, value)
	case "NpcBot.MaxBots":
		return setUint32(&c.NPCBots.MaxBots, key, value)
	case "NpcBot.MaxBotsPerClass":
		return setUint32(&c.NPCBots.MaxBotsPerClass, key, value)
	case "NpcBot.BaseFollowDistance":
		return setUint32(&c.NPCBots.BaseFollowDistance, key, value)
	case "NpcBot.XpReduction":
		return setUint32(&c.NPCBots.XPReduction, key, value)
	case "NpcBot.HealTargetIconsMask":
		return setUint32(&c.NPCBots.HealTargetIconsMask, key, value)
	case "NpcBot.TankTargetIconMask":
		return setUint32(&c.NPCBots.TankTargetIconMask, key, value)
	case "NpcBot.DPSTargetIconMask":
		return setUint32(&c.NPCBots.DPSTargetIconMask, key, value)
	case "NpcBot.Mult.Damage.Physical":
		return setFloat64(&c.NPCBots.DamagePhysicalMultiplier, key, value)
	case "NpcBot.Mult.Damage.Spell":
		return setFloat64(&c.NPCBots.DamageSpellMultiplier, key, value)
	case "NpcBot.Mult.Healing":
		return setFloat64(&c.NPCBots.HealingMultiplier, key, value)
	case "NpcBot.Enable.Dungeon":
		return setBool(&c.NPCBots.EnableDungeon, key, value)
	case "NpcBot.Enable.Raid":
		return setBool(&c.NPCBots.EnableRaid, key, value)
	case "NpcBot.Enable.BG":
		return setBool(&c.NPCBots.EnableBG, key, value)
	case "NpcBot.Enable.Arena":
		return setBool(&c.NPCBots.EnableArena, key, value)
	case "NpcBot.Enable.DungeonFinder":
		return setBool(&c.NPCBots.EnableDungeonFinder, key, value)
	case "NpcBot.Limit.Dungeon":
		return setBool(&c.NPCBots.LimitDungeon, key, value)
	case "NpcBot.Limit.Raid":
		return setBool(&c.NPCBots.LimitRaid, key, value)
	case "NpcBot.Cost":
		return setUint64(&c.NPCBots.Cost, key, value)
	case "NpcBot.UpdateDelay.Base":
		return setUint32(&c.NPCBots.UpdateDelayBase, key, value)
	case "NpcBot.OwnershipExpireTime":
		return setUint32(&c.NPCBots.OwnershipExpireTime, key, value)
	case "NpcBot.PvP":
		return setBool(&c.NPCBots.PvP, key, value)
	case "NpcBot.Movements.InterruptFood":
		return setBool(&c.NPCBots.MovementInterruptFood, key, value)
	case "NpcBot.EquipmentDisplay.Enable":
		return setBool(&c.NPCBots.EquipmentDisplayEnable, key, value)
	case "NpcBot.EquipmentDisplay.ShowCloak":
		return setBool(&c.NPCBots.ShowCloak, key, value)
	case "NpcBot.EquipmentDisplay.ShowHelm":
		return setBool(&c.NPCBots.ShowHelm, key, value)
	case "NpcBot.NewClasses.Blademaster.Enable":
		return setBool(&c.NPCBots.BlademasterEnable, key, value)
	case "NpcBot.NewClasses.ObsidianDestroyer.Enable":
		return setBool(&c.NPCBots.ObsidianDestroyerEnable, key, value)
	case "NpcBot.NewClasses.Archmage.Enable":
		return setBool(&c.NPCBots.ArchmageEnable, key, value)
	case "NpcBot.NewClasses.Dreadlord.Enable":
		return setBool(&c.NPCBots.DreadlordEnable, key, value)
	case "NpcBot.NewClasses.SpellBreaker.Enable":
		return setBool(&c.NPCBots.SpellBreakerEnable, key, value)
	case "NpcBot.NewClasses.DarkRanger.Enable":
		return setBool(&c.NPCBots.DarkRangerEnable, key, value)
	case "NpcBot.Stats.Limits.Enable":
		return setBool(&c.NPCBots.StatsLimitsEnable, key, value)
	case "NpcBot.Stats.Limits.Dodge":
		return setFloat64(&c.NPCBots.StatLimitDodge, key, value)
	case "NpcBot.Stats.Limits.Parry":
		return setFloat64(&c.NPCBots.StatLimitParry, key, value)
	case "NpcBot.Stats.Limits.Block":
		return setFloat64(&c.NPCBots.StatLimitBlock, key, value)
	case "NpcBot.Stats.Limits.Crit":
		return setFloat64(&c.NPCBots.StatLimitCrit, key, value)
	default:
		c.UnrecognizedKeys = append(c.UnrecognizedKeys, key)
	}
	return nil
}

func setInt(dst *int, key, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s must be an integer: %w", key, err)
	}
	*dst = n
	return nil
}

func setBool(dst *bool, key, value string) error {
	b, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("%s must be boolean: %w", key, err)
	}
	*dst = b
	return nil
}

func setUint32(dst *uint32, key, value string) error {
	n, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return fmt.Errorf("%s must be an unsigned integer: %w", key, err)
	}
	*dst = uint32(n)
	return nil
}

func setUint64(dst *uint64, key, value string) error {
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fmt.Errorf("%s must be an unsigned integer: %w", key, err)
	}
	*dst = n
	return nil
}

func setFloat64(dst *float64, key, value string) error {
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("%s must be a number: %w", key, err)
	}
	*dst = n
	return nil
}

func (c Config) Validate() error {
	if c.Backend != "sqlite" && c.Backend != "mysql" && c.Backend != "mariadb" {
		return errors.New("Database.Backend must be sqlite, mysql, or mariadb")
	}
	if c.RealmServerPort < 1 || c.RealmServerPort > 65535 || c.WorldServerPort < 1 || c.WorldServerPort > 65535 {
		return errors.New("server ports must be between 1 and 65535")
	}
	if c.CharactersPerRealm < 1 || c.CharactersPerRealm > 10 {
		return errors.New("CharactersPerRealm must be between 1 and 10")
	}
	if c.CharactersPerAccount < c.CharactersPerRealm {
		return errors.New("CharactersPerAccount cannot be less than CharactersPerRealm")
	}
	if c.DeathKnightsPerRealm > 10 {
		return errors.New("DeathKnightsPerRealm must be between 0 and 10")
	}
	if c.NPCBots.DamagePhysicalMultiplier < 0.1 || c.NPCBots.DamagePhysicalMultiplier > 10 || c.NPCBots.DamageSpellMultiplier < 0.1 || c.NPCBots.DamageSpellMultiplier > 10 || c.NPCBots.HealingMultiplier < 0.1 || c.NPCBots.HealingMultiplier > 10 {
		return errors.New("NpcBot damage and healing multipliers must be between 0.1 and 10")
	}
	if c.VisibilityDistanceContinents <= 0 || c.VisibilityDistanceContinents > 1000 {
		return errors.New("Visibility.Distance.Continents must be between 0 and 1000")
	}
	return nil
}
