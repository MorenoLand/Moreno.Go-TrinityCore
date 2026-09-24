package world

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

// Achievement system core, mirroring the reference:
//   - AchievementMgr::SendAllAchievementData (login dump: earned list, -1,
//     progress list, -1 via SMSG_ALL_ACHIEVEMENT_DATA)
//   - WorldSession::HandleQueryInspectAchievements -> RespondInspectAchievements
//     (packed target GUID + earned/progress blocks, Achievements of others
//     show only completed achievements)
//   - AchievementMgr::SetCriteriaProgress + CriteriaUpdate wire format
//     (AchievementPackets.cpp): criteria id, packed quantity, packed player
//     GUID, flags, date, elapsed, creation time
//   - AchievementMgr::CompletedCriteriaForAchievement: a criterion completes
//     at progress >= criteria.Quantity (field 4 of Achievement_Criteria.dbc)
//   - AchievementMgr::CompletedAchievement: SMSG_ACHIEVEMENT_EARNED to the
//     player and nearby players, persisted to character_achievement
//
// DBC layouts per DBCStructure.h: Achievement.dbc fields 0/1/2/38/39/41/60/61
// (ID, Faction, InstanceID, Category, Points, Flags, MinimumCriteria,
// SharesCriteria); Achievement_Criteria.dbc fields 0-4 (ID, AchievementID,
// Type, Asset, Quantity), 26-29 (Flags, StartEvent, StartAsset, StartTimer).

// CriteriaTypeCount defines the full range of achievement criteria types (0..123).
const CriteriaTypeCount = 124
const achievementFlagHidden uint32 = 0x00000002

// Achievement criteria types (0..123) matching TrinityCore 3.3.5 and Achievement_Criteria.dbc.
const (
	criteriaTypeKillCreature            = 0   // ACHIEVEMENT_CRITERIA_TYPE_KILL_CREATURE
	criteriaTypeWinBG                   = 1   // ACHIEVEMENT_CRITERIA_TYPE_WIN_BG
	criteriaTypeUnused2                 = 2   // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_2
	criteriaTypeUnused3                 = 3   // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_3
	criteriaTypeUnused4                 = 4   // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_4
	criteriaTypeReachLevel              = 5   // ACHIEVEMENT_CRITERIA_TYPE_REACH_LEVEL
	criteriaTypeUnused6                 = 6   // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_6
	criteriaTypeReachSkillLevel         = 7   // ACHIEVEMENT_CRITERIA_TYPE_REACH_SKILL_LEVEL
	criteriaTypeCompleteAchievement     = 8   // ACHIEVEMENT_CRITERIA_TYPE_COMPLETE_ACHIEVEMENT
	criteriaTypeQuestCount              = 9   // ACHIEVEMENT_CRITERIA_TYPE_COMPLETE_QUEST_COUNT
	criteriaTypeCompleteDailyQuestDaily = 10  // ACHIEVEMENT_CRITERIA_TYPE_COMPLETE_DAILY_QUEST_DAILY
	criteriaTypeCompleteQuestsInZone    = 11  // ACHIEVEMENT_CRITERIA_TYPE_COMPLETE_QUESTS_IN_ZONE
	criteriaTypeUnused12                = 12  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_12
	criteriaTypeDamageDone              = 13  // ACHIEVEMENT_CRITERIA_TYPE_DAMAGE_DONE
	criteriaTypeCompleteDailyQuest      = 14  // ACHIEVEMENT_CRITERIA_TYPE_COMPLETE_DAILY_QUEST
	criteriaTypeCompleteBattleground    = 15  // ACHIEVEMENT_CRITERIA_TYPE_COMPLETE_BATTLEGROUND
	criteriaTypeDeathAtMap              = 16  // ACHIEVEMENT_CRITERIA_TYPE_DEATH_AT_MAP
	criteriaTypeDeath                   = 17  // ACHIEVEMENT_CRITERIA_TYPE_DEATH
	criteriaTypeDeathInDungeon          = 18  // ACHIEVEMENT_CRITERIA_TYPE_DEATH_IN_DUNGEON
	criteriaTypeCompleteRaid            = 19  // ACHIEVEMENT_CRITERIA_TYPE_COMPLETE_RAID
	criteriaTypeKilledByCreature        = 20  // ACHIEVEMENT_CRITERIA_TYPE_KILLED_BY_CREATURE
	criteriaTypeUnused21                = 21  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_21
	criteriaTypeUnused22                = 22  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_22
	criteriaTypeKilledByPlayer          = 23  // ACHIEVEMENT_CRITERIA_TYPE_KILLED_BY_PLAYER
	criteriaTypeFallWithoutDying        = 24  // ACHIEVEMENT_CRITERIA_TYPE_FALL_WITHOUT_DYING
	criteriaTypeUnused25                = 25  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_25
	criteriaTypeDeathsFrom              = 26  // ACHIEVEMENT_CRITERIA_TYPE_DEATHS_FROM
	criteriaTypeCompleteQuest           = 27  // ACHIEVEMENT_CRITERIA_TYPE_COMPLETE_QUEST
	criteriaTypeBeSpellTarget           = 28  // ACHIEVEMENT_CRITERIA_TYPE_BE_SPELL_TARGET
	criteriaTypeCastSpell               = 29  // ACHIEVEMENT_CRITERIA_TYPE_CAST_SPELL
	criteriaTypeBGObjective             = 30  // ACHIEVEMENT_CRITERIA_TYPE_BG_OBJECTIVE_CAPTURE
	criteriaTypeHKAtArea                = 31  // ACHIEVEMENT_CRITERIA_TYPE_HONORABLE_KILL_AT_AREA
	criteriaTypeWinArena                = 32  // ACHIEVEMENT_CRITERIA_TYPE_WIN_ARENA
	criteriaTypePlayArena               = 33  // ACHIEVEMENT_CRITERIA_TYPE_PLAY_ARENA
	criteriaTypeLearnSpell              = 34  // ACHIEVEMENT_CRITERIA_TYPE_LEARN_SPELL
	criteriaTypeHonorableKill           = 35  // ACHIEVEMENT_CRITERIA_TYPE_HONORABLE_KILL
	criteriaTypeOwnItem                 = 36  // ACHIEVEMENT_CRITERIA_TYPE_OWN_ITEM
	criteriaTypeWinRatedArena           = 37  // ACHIEVEMENT_CRITERIA_TYPE_WIN_RATED_ARENA
	criteriaTypeHighestTeamRating       = 38  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_TEAM_RATING
	criteriaTypeHighestPersonalRating   = 39  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_PERSONAL_RATING
	criteriaTypeLearnSkillLevel         = 40  // ACHIEVEMENT_CRITERIA_TYPE_LEARN_SKILL_LEVEL
	criteriaTypeUseItem                 = 41  // ACHIEVEMENT_CRITERIA_TYPE_USE_ITEM
	criteriaTypeLootItem                = 42  // ACHIEVEMENT_CRITERIA_TYPE_LOOT_ITEM
	criteriaTypeExplore                 = 43  // ACHIEVEMENT_CRITERIA_TYPE_EXPLORE_AREA
	criteriaTypeOwnRank                 = 44  // ACHIEVEMENT_CRITERIA_TYPE_OWN_RANK
	criteriaTypeBuyBankSlot             = 45  // ACHIEVEMENT_CRITERIA_TYPE_BUY_BANK_SLOT
	criteriaTypeGainReputation          = 46  // ACHIEVEMENT_CRITERIA_TYPE_GAIN_REPUTATION
	criteriaTypeExaltedRep              = 47  // ACHIEVEMENT_CRITERIA_TYPE_GAIN_EXALTED_REPUTATION
	criteriaTypeVisitBarberShop         = 48  // ACHIEVEMENT_CRITERIA_TYPE_VISIT_BARBER_SHOP
	criteriaTypeEquipEpicItem           = 49  // ACHIEVEMENT_CRITERIA_TYPE_EQUIP_EPIC_ITEM
	criteriaTypeRollNeed                = 50  // ACHIEVEMENT_CRITERIA_TYPE_ROLL_NEED_ON_LOOT
	criteriaTypeRollGreed               = 51  // ACHIEVEMENT_CRITERIA_TYPE_ROLL_GREED_ON_LOOT
	criteriaTypeHKClass                 = 52  // ACHIEVEMENT_CRITERIA_TYPE_HK_CLASS
	criteriaTypeHKRace                  = 53  // ACHIEVEMENT_CRITERIA_TYPE_HK_RACE
	criteriaTypeDoEmote                 = 54  // ACHIEVEMENT_CRITERIA_TYPE_DO_EMOTE
	criteriaTypeHealingDone             = 55  // ACHIEVEMENT_CRITERIA_TYPE_HEALING_DONE
	criteriaTypeGetKillingBlows         = 56  // ACHIEVEMENT_CRITERIA_TYPE_GET_KILLING_BLOWS
	criteriaTypeEquipItem               = 57  // ACHIEVEMENT_CRITERIA_TYPE_EQUIP_ITEM
	criteriaTypeUnused58                = 58  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_58
	criteriaTypeMoneyFromVendor         = 59  // ACHIEVEMENT_CRITERIA_TYPE_MONEY_FROM_VENDORS
	criteriaTypeGoldSpentForTalents     = 60  // ACHIEVEMENT_CRITERIA_TYPE_GOLD_SPENT_FOR_TALENTS
	criteriaTypeTalentResets            = 61  // ACHIEVEMENT_CRITERIA_TYPE_NUMBER_OF_TALENT_RESETS
	criteriaTypeMoneyFromQuest          = 62  // ACHIEVEMENT_CRITERIA_TYPE_MONEY_FROM_QUEST_REWARD
	criteriaTypeGoldSpentTravel         = 63  // ACHIEVEMENT_CRITERIA_TYPE_GOLD_SPENT_FOR_TRAVELLING
	criteriaTypeUnused64                = 64  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_64
	criteriaTypeGoldSpentAtBarber       = 65  // ACHIEVEMENT_CRITERIA_TYPE_GOLD_SPENT_AT_BARBER
	criteriaTypeGoldSpentForMail        = 66  // ACHIEVEMENT_CRITERIA_TYPE_GOLD_SPENT_FOR_MAIL
	criteriaTypeLootMoney               = 67  // ACHIEVEMENT_CRITERIA_TYPE_LOOT_MONEY
	criteriaTypeUseGameObject           = 68  // ACHIEVEMENT_CRITERIA_TYPE_USE_GAMEOBJECT
	criteriaTypeBeSpellTarget2          = 69  // ACHIEVEMENT_CRITERIA_TYPE_BE_SPELL_TARGET2
	criteriaTypeSpecialPvPKill          = 70  // ACHIEVEMENT_CRITERIA_TYPE_SPECIAL_PVP_KILL
	criteriaTypeUnused71                = 71  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_71
	criteriaTypeFishInGameObject        = 72  // ACHIEVEMENT_CRITERIA_TYPE_FISH_IN_GAMEOBJECT
	criteriaTypeMalGanisDefeated        = 73  // ACHIEVEMENT_CRITERIA_TYPE_MAL_GANIS_DEFEATED
	criteriaTypeOnLogin                 = 74  // ACHIEVEMENT_CRITERIA_TYPE_ON_LOGIN
	criteriaTypeLearnSkillLineSpells    = 75  // ACHIEVEMENT_CRITERIA_TYPE_LEARN_SKILLLINE_SPELLS
	criteriaTypeWinDuel                 = 76  // ACHIEVEMENT_CRITERIA_TYPE_WIN_DUEL
	criteriaTypeLoseDuel                = 77  // ACHIEVEMENT_CRITERIA_TYPE_LOSE_DUEL
	criteriaTypeKillCreatureType        = 78  // ACHIEVEMENT_CRITERIA_TYPE_KILL_CREATURE_TYPE
	criteriaTypeUnused79                = 79  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_79
	criteriaTypeGoldEarnedAuctions      = 80  // ACHIEVEMENT_CRITERIA_TYPE_GOLD_EARNED_BY_AUCTIONS
	criteriaTypeUnused81                = 81  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_81
	criteriaTypeCreateAuction           = 82  // ACHIEVEMENT_CRITERIA_TYPE_CREATE_AUCTION
	criteriaTypeHighestAuctionBid       = 83  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_AUCTION_BID
	criteriaTypeWonAuctions             = 84  // ACHIEVEMENT_CRITERIA_TYPE_WON_AUCTIONS
	criteriaTypeHighestAuctionSold      = 85  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_AUCTION_SOLD
	criteriaTypeHighestGoldValue        = 86  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_GOLD_VALUE_OWNED
	criteriaTypeReveredRep              = 87  // ACHIEVEMENT_CRITERIA_TYPE_GAIN_REVERED_REPUTATION
	criteriaTypeHonoredRep              = 88  // ACHIEVEMENT_CRITERIA_TYPE_GAIN_HONORED_REPUTATION
	criteriaTypeKnownFactions           = 89  // ACHIEVEMENT_CRITERIA_TYPE_KNOWN_FACTIONS
	criteriaTypeLootEpicItem            = 90  // ACHIEVEMENT_CRITERIA_TYPE_LOOT_EPIC_ITEM
	criteriaTypeReceiveEpicItem         = 91  // ACHIEVEMENT_CRITERIA_TYPE_RECEIVE_EPIC_ITEM
	criteriaTypeUnused92                = 92  // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_92
	criteriaTypeRollNeedCount           = 93  // ACHIEVEMENT_CRITERIA_TYPE_ROLL_NEED
	criteriaTypeRollGreedCount          = 94  // ACHIEVEMENT_CRITERIA_TYPE_ROLL_GREED
	criteriaTypeHighestHealth           = 95  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_HEALTH
	criteriaTypeHighestPower            = 96  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_POWER
	criteriaTypeHighestStat             = 97  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_STAT
	criteriaTypeHighestSpellpower       = 98  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_SPELLPOWER
	criteriaTypeHighestArmor            = 99  // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_ARMOR
	criteriaTypeHighestRating           = 100 // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_RATING
	criteriaTypeHighestHitDealt         = 101 // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_HIT_DEALT
	criteriaTypeHighestHitReceived      = 102 // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_HIT_RECEIVED
	criteriaTypeTotalDamageReceived     = 103 // ACHIEVEMENT_CRITERIA_TYPE_TOTAL_DAMAGE_RECEIVED
	criteriaTypeHighestHealCasted       = 104 // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_HEAL_CAST
	criteriaTypeTotalHealingReceived    = 105 // ACHIEVEMENT_CRITERIA_TYPE_TOTAL_HEALING_RECEIVED
	criteriaTypeHighestHealingRecv      = 106 // ACHIEVEMENT_CRITERIA_TYPE_HIGHEST_HEALING_RECEIVED
	criteriaTypeQuestAbandoned          = 107 // ACHIEVEMENT_CRITERIA_TYPE_QUEST_ABANDONED
	criteriaTypeFlightPathsTaken        = 108 // ACHIEVEMENT_CRITERIA_TYPE_FLIGHT_PATHS_TAKEN
	criteriaTypeLootType                = 109 // ACHIEVEMENT_CRITERIA_TYPE_LOOT_TYPE
	criteriaTypeCastSpell2              = 110 // ACHIEVEMENT_CRITERIA_TYPE_CAST_SPELL2
	criteriaTypeUnused111               = 111 // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_111
	criteriaTypeLearnSkillLine          = 112 // ACHIEVEMENT_CRITERIA_TYPE_LEARN_SKILL_LINE
	criteriaTypeEarnHonorableKill       = 113 // ACHIEVEMENT_CRITERIA_TYPE_EARN_HONORABLE_KILL
	criteriaTypeAcceptedSummonings      = 114 // ACHIEVEMENT_CRITERIA_TYPE_ACCEPTED_SUMMONINGS
	criteriaTypeEarnAchievementPoints   = 115 // ACHIEVEMENT_CRITERIA_TYPE_EARN_ACHIEVEMENT_POINTS
	criteriaTypeUnused116               = 116 // ACHIEVEMENT_CRITERIA_TYPE_UNUSED_116
	criteriaTypeRollDisenchant          = 117 // ACHIEVEMENT_CRITERIA_TYPE_ROLL_DISENCHANT
	criteriaTypeLFGAnyRole              = 118 // ACHIEVEMENT_CRITERIA_TYPE_LFG_ANY_ROLE
	criteriaTypeUseLFDToGroup           = 119 // ACHIEVEMENT_CRITERIA_TYPE_USE_LFD_TO_GROUP_WITH_PLAYERS
	criteriaTypeLFGVoteKick             = 120 // ACHIEVEMENT_CRITERIA_TYPE_LFG_VOTE_KICK
	criteriaTypeLFGDungeonReward        = 121 // ACHIEVEMENT_CRITERIA_TYPE_LFG_DUNGEON_REWARD
	criteriaTypeLFGCompletion           = 122 // ACHIEVEMENT_CRITERIA_TYPE_LFG_COMPLETION
	criteriaTypeLFGAbandon              = 123 // ACHIEVEMENT_CRITERIA_TYPE_LFG_ABANDON
)

var criteriaTypeNames = [CriteriaTypeCount]string{
	criteriaTypeKillCreature:            "KillCreature",
	criteriaTypeWinBG:                   "WinBG",
	criteriaTypeUnused2:                 "Unused2",
	criteriaTypeUnused3:                 "Unused3",
	criteriaTypeUnused4:                 "Unused4",
	criteriaTypeReachLevel:              "ReachLevel",
	criteriaTypeUnused6:                 "Unused6",
	criteriaTypeReachSkillLevel:         "ReachSkillLevel",
	criteriaTypeCompleteAchievement:     "CompleteAchievement",
	criteriaTypeQuestCount:              "QuestCount",
	criteriaTypeCompleteDailyQuestDaily: "CompleteDailyQuestDaily",
	criteriaTypeCompleteQuestsInZone:    "CompleteQuestsInZone",
	criteriaTypeUnused12:                "Unused12",
	criteriaTypeDamageDone:              "DamageDone",
	criteriaTypeCompleteDailyQuest:      "CompleteDailyQuest",
	criteriaTypeCompleteBattleground:    "CompleteBattleground",
	criteriaTypeDeathAtMap:              "DeathAtMap",
	criteriaTypeDeath:                   "Death",
	criteriaTypeDeathInDungeon:          "DeathInDungeon",
	criteriaTypeCompleteRaid:            "CompleteRaid",
	criteriaTypeKilledByCreature:        "KilledByCreature",
	criteriaTypeUnused21:                "Unused21",
	criteriaTypeUnused22:                "Unused22",
	criteriaTypeKilledByPlayer:          "KilledByPlayer",
	criteriaTypeFallWithoutDying:        "FallWithoutDying",
	criteriaTypeUnused25:                "Unused25",
	criteriaTypeDeathsFrom:              "DeathsFrom",
	criteriaTypeCompleteQuest:           "CompleteQuest",
	criteriaTypeBeSpellTarget:           "BeSpellTarget",
	criteriaTypeCastSpell:               "CastSpell",
	criteriaTypeBGObjective:             "BGObjective",
	criteriaTypeHKAtArea:                "HKAtArea",
	criteriaTypeWinArena:                "WinArena",
	criteriaTypePlayArena:               "PlayArena",
	criteriaTypeLearnSpell:              "LearnSpell",
	criteriaTypeHonorableKill:           "HonorableKill",
	criteriaTypeOwnItem:                 "OwnItem",
	criteriaTypeWinRatedArena:           "WinRatedArena",
	criteriaTypeHighestTeamRating:       "HighestTeamRating",
	criteriaTypeHighestPersonalRating:   "HighestPersonalRating",
	criteriaTypeLearnSkillLevel:         "LearnSkillLevel",
	criteriaTypeUseItem:                 "UseItem",
	criteriaTypeLootItem:                "LootItem",
	criteriaTypeExplore:                 "Explore",
	criteriaTypeOwnRank:                 "OwnRank",
	criteriaTypeBuyBankSlot:             "BuyBankSlot",
	criteriaTypeGainReputation:          "GainReputation",
	criteriaTypeExaltedRep:              "ExaltedRep",
	criteriaTypeVisitBarberShop:         "VisitBarberShop",
	criteriaTypeEquipEpicItem:           "EquipEpicItem",
	criteriaTypeRollNeed:                "RollNeed",
	criteriaTypeRollGreed:               "RollGreed",
	criteriaTypeHKClass:                 "HKClass",
	criteriaTypeHKRace:                  "HKRace",
	criteriaTypeDoEmote:                 "DoEmote",
	criteriaTypeHealingDone:             "HealingDone",
	criteriaTypeGetKillingBlows:         "GetKillingBlows",
	criteriaTypeEquipItem:               "EquipItem",
	criteriaTypeUnused58:                "Unused58",
	criteriaTypeMoneyFromVendor:         "MoneyFromVendor",
	criteriaTypeGoldSpentForTalents:     "GoldSpentForTalents",
	criteriaTypeTalentResets:            "TalentResets",
	criteriaTypeMoneyFromQuest:          "MoneyFromQuest",
	criteriaTypeGoldSpentTravel:         "GoldSpentTravel",
	criteriaTypeUnused64:                "Unused64",
	criteriaTypeGoldSpentAtBarber:       "GoldSpentAtBarber",
	criteriaTypeGoldSpentForMail:        "GoldSpentForMail",
	criteriaTypeLootMoney:               "LootMoney",
	criteriaTypeUseGameObject:           "UseGameObject",
	criteriaTypeBeSpellTarget2:          "BeSpellTarget2",
	criteriaTypeSpecialPvPKill:          "SpecialPvPKill",
	criteriaTypeUnused71:                "Unused71",
	criteriaTypeFishInGameObject:        "FishInGameObject",
	criteriaTypeMalGanisDefeated:        "MalGanisDefeated",
	criteriaTypeOnLogin:                 "OnLogin",
	criteriaTypeLearnSkillLineSpells:    "LearnSkillLineSpells",
	criteriaTypeWinDuel:                 "WinDuel",
	criteriaTypeLoseDuel:                "LoseDuel",
	criteriaTypeKillCreatureType:        "KillCreatureType",
	criteriaTypeUnused79:                "Unused79",
	criteriaTypeGoldEarnedAuctions:      "GoldEarnedAuctions",
	criteriaTypeUnused81:                "Unused81",
	criteriaTypeCreateAuction:           "CreateAuction",
	criteriaTypeHighestAuctionBid:       "HighestAuctionBid",
	criteriaTypeWonAuctions:             "WonAuctions",
	criteriaTypeHighestAuctionSold:      "HighestAuctionSold",
	criteriaTypeHighestGoldValue:        "HighestGoldValue",
	criteriaTypeReveredRep:              "ReveredRep",
	criteriaTypeHonoredRep:              "HonoredRep",
	criteriaTypeKnownFactions:           "KnownFactions",
	criteriaTypeLootEpicItem:            "LootEpicItem",
	criteriaTypeReceiveEpicItem:         "ReceiveEpicItem",
	criteriaTypeUnused92:                "Unused92",
	criteriaTypeRollNeedCount:           "RollNeedCount",
	criteriaTypeRollGreedCount:          "RollGreedCount",
	criteriaTypeHighestHealth:           "HighestHealth",
	criteriaTypeHighestPower:            "HighestPower",
	criteriaTypeHighestStat:             "HighestStat",
	criteriaTypeHighestSpellpower:       "HighestSpellpower",
	criteriaTypeHighestArmor:            "HighestArmor",
	criteriaTypeHighestRating:           "HighestRating",
	criteriaTypeHighestHitDealt:         "HighestHitDealt",
	criteriaTypeHighestHitReceived:      "HighestHitReceived",
	criteriaTypeTotalDamageReceived:     "TotalDamageReceived",
	criteriaTypeHighestHealCasted:       "HighestHealCasted",
	criteriaTypeTotalHealingReceived:    "TotalHealingReceived",
	criteriaTypeHighestHealingRecv:      "HighestHealingRecv",
	criteriaTypeQuestAbandoned:          "QuestAbandoned",
	criteriaTypeFlightPathsTaken:        "FlightPathsTaken",
	criteriaTypeLootType:                "LootType",
	criteriaTypeCastSpell2:              "CastSpell2",
	criteriaTypeUnused111:               "Unused111",
	criteriaTypeLearnSkillLine:          "LearnSkillLine",
	criteriaTypeEarnHonorableKill:       "EarnHonorableKill",
	criteriaTypeAcceptedSummonings:      "AcceptedSummonings",
	criteriaTypeEarnAchievementPoints:   "EarnAchievementPoints",
	criteriaTypeUnused116:               "Unused116",
	criteriaTypeRollDisenchant:          "RollDisenchant",
	criteriaTypeLFGAnyRole:              "LFGAnyRole",
	criteriaTypeUseLFDToGroup:           "UseLFDToGroup",
	criteriaTypeLFGVoteKick:             "LFGVoteKick",
	criteriaTypeLFGDungeonReward:        "LFGDungeonReward",
	criteriaTypeLFGCompletion:           "LFGCompletion",
	criteriaTypeLFGAbandon:              "LFGAbandon",
}

// CriteriaCondition constants matching DBCEnums.h:96-108.
const (
	criteriaConditionNone       = 0
	criteriaConditionNoDeath    = 1 // reset progress on death
	criteriaConditionUnk2       = 2
	criteriaConditionBGMap      = 3 // requires specific map
	criteriaConditionNoLose     = 4 // reset progress on arena loss
	criteriaConditionNoSpellHit = 9
	criteriaConditionNotInGroup = 10 // requires player not to be in group
	criteriaConditionMax        = 14
)

// CriteriaDataType constants matching AchievementMgr.h:49-80 (achievement_criteria_data table).
const (
	criteriaDataTypeNone              = 0
	criteriaDataTypeTCreature         = 1
	criteriaDataTypeTPlayerClassRace  = 2
	criteriaDataTypeTPlayerLessHealth = 3
	criteriaDataTypeTPlayerDead       = 4
	criteriaDataTypeSAura             = 5
	criteriaDataTypeSArea             = 6
	criteriaDataTypeTAura             = 7
	criteriaDataTypeValue             = 8
	criteriaDataTypeTLevel            = 9
	criteriaDataTypeTGender           = 10
	criteriaDataTypeScript            = 11
	criteriaDataTypeMapDifficulty     = 12
	criteriaDataTypeMapPlayerCount    = 13
	criteriaDataTypeTTeam             = 14
	criteriaDataTypeSDrunk            = 15
	criteriaDataTypeHoliday           = 16
	criteriaDataTypeBGLossTeamScore   = 17
	criteriaDataTypeInstanceScript    = 18
	criteriaDataTypeSEquippedItem     = 19
	criteriaDataTypeMapID             = 20
	criteriaDataTypeSPlayerClassRace  = 21
	criteriaDataTypeNthBirthday       = 22
	criteriaDataTypeSKnownTitle       = 23
	criteriaDataTypeSItemQuality      = 25
)

// CriteriaTypeName returns the canonical name for an achievement criteria type.
func CriteriaTypeName(cType uint32) string {
	if cType < CriteriaTypeCount {
		return criteriaTypeNames[cType]
	}
	return fmt.Sprintf("UnknownCriteriaType(%d)", cType)
}

type achievementEntry struct {
	ID              uint32
	Faction         int32 // -1 any, 0 horde, 1 alliance
	Category        uint32
	Points          uint32
	Flags           uint32
	MinimumCriteria uint32
}

type achievementCriteriaEntry struct {
	ID            uint32
	AchievementID uint32
	Type          uint32
	Asset         uint32
	Quantity      uint32 // required count
	ReqType1      uint32 // AdditionalRequirements[0].Type (DBC field 5)
	ReqAsset1     uint32 // AdditionalRequirements[0].Asset (DBC field 6)
	ReqType2      uint32 // AdditionalRequirements[1].Type (DBC field 7)
	ReqAsset2     uint32 // AdditionalRequirements[1].Asset (DBC field 8)
	StartEvent    uint32 // AchievementCriteriaTimedTypes (DBC field 27)
	StartAsset    uint32 // DBC field 28
	StartTimer    uint32 // seconds (DBC field 29)
}

type criteriaDataEntry struct {
	Type       uint32
	Value1     uint32
	Value2     uint32
	ScriptName string
}

type criteriaProgressState struct {
	CriteriaID uint32
	Counter    uint32
	Date       uint32
}

type achievementRuntime struct {
	mu            sync.RWMutex
	byTypeAsset   map[uint64][]achievementCriteriaEntry // key: type<<32 | asset
	byTimedEvent  map[uint64][]achievementCriteriaEntry // key: startEvent<<32 | startAsset
	byType        map[uint32][]achievementCriteriaEntry
	byCondition   map[uint32][]achievementCriteriaEntry // key: condition type
	criteriaData  map[uint32][]criteriaDataEntry        // key: criteria_id
	exploreByZone map[uint32][]uint32                   // zone id -> criteria ids (type 43)
	byID          map[uint32]achievementCriteriaEntry
	byAchieve     map[uint32][]achievementCriteriaEntry
	achieveByID   map[uint32]achievementEntry
	loaded        bool
}

var achievementIndex = &achievementRuntime{
	byTypeAsset:   make(map[uint64][]achievementCriteriaEntry),
	byTimedEvent:  make(map[uint64][]achievementCriteriaEntry),
	byType:        make(map[uint32][]achievementCriteriaEntry),
	byCondition:   make(map[uint32][]achievementCriteriaEntry),
	criteriaData:  make(map[uint32][]criteriaDataEntry),
	exploreByZone: make(map[uint32][]uint32),
	byID:          make(map[uint32]achievementCriteriaEntry),
	byAchieve:     make(map[uint32][]achievementCriteriaEntry),
	achieveByID:   make(map[uint32]achievementEntry),
}

func typeAssetKey(criterionType, asset uint32) uint64 {
	return uint64(criterionType)<<32 | uint64(asset)
}

// loadAchievementIndex builds the criteria/achievement index from the DBC
// stores and database once per process.
func (s *Server) loadAchievementIndex() {
	achievementIndex.mu.Lock()
	defer achievementIndex.mu.Unlock()
	if achievementIndex.loaded {
		return
	}
	if s.Data != nil {
		if file, err := s.Data.File("Achievement_Criteria"); err == nil {
			for i := 0; i < file.Records(); i++ {
				record, err := file.Record(i)
				if err != nil {
					continue
				}
				id, err := record.Uint32(0)
				if err != nil {
					continue
				}
				achievementID, err := record.Uint32(1)
				if err != nil {
					continue
				}
				criterionType, err := record.Uint32(2)
				if err != nil {
					continue
				}
				asset, err := record.Uint32(3)
				if err != nil {
					continue
				}
				quantity, err := record.Uint32(4)
				if err != nil {
					continue
				}
				reqType1, _ := record.Uint32(5)
				reqAsset1, _ := record.Uint32(6)
				reqType2, _ := record.Uint32(7)
				reqAsset2, _ := record.Uint32(8)
				startEvent, _ := record.Uint32(27)
				startAsset, _ := record.Uint32(28)
				startTimer, _ := record.Uint32(29)
				entry := achievementCriteriaEntry{
					ID:            id,
					AchievementID: achievementID,
					Type:          criterionType,
					Asset:         asset,
					Quantity:      quantity,
					ReqType1:      reqType1,
					ReqAsset1:     reqAsset1,
					ReqType2:      reqType2,
					ReqAsset2:     reqAsset2,
					StartEvent:    startEvent,
					StartAsset:    startAsset,
					StartTimer:    startTimer,
				}
				key := typeAssetKey(criterionType, asset)
				achievementIndex.byTypeAsset[key] = append(achievementIndex.byTypeAsset[key], entry)
				achievementIndex.byType[criterionType] = append(achievementIndex.byType[criterionType], entry)
				if reqType1 != 0 {
					achievementIndex.byCondition[reqType1] = append(achievementIndex.byCondition[reqType1], entry)
				}
				if reqType2 != 0 && (reqType2 != reqType1 || reqAsset2 != reqAsset1) {
					achievementIndex.byCondition[reqType2] = append(achievementIndex.byCondition[reqType2], entry)
				}
				if startEvent != 0 {
					timedKey := typeAssetKey(startEvent, startAsset)
					achievementIndex.byTimedEvent[timedKey] = append(achievementIndex.byTimedEvent[timedKey], entry)
				}
				achievementIndex.byID[id] = entry
				achievementIndex.byAchieve[achievementID] = append(achievementIndex.byAchieve[achievementID], entry)
			}
			for _, entry := range achievementIndex.byType[criteriaTypeExplore] {
				areas, found, err := s.Data.WorldMapOverlayAreas(entry.Asset)
				if err != nil || !found {
					continue
				}
				for _, area := range areas {
					if area != 0 {
						achievementIndex.exploreByZone[area] = append(achievementIndex.exploreByZone[area], entry.ID)
					}
				}
			}
		}
		if af, err := s.Data.File("Achievement"); err == nil {
			for i := 0; i < af.Records(); i++ {
				record, err := af.Record(i)
				if err != nil {
					continue
				}
				id, err := record.Uint32(0)
				if err != nil {
					continue
				}
				faction, _ := record.Int32(1)
				category, _ := record.Uint32(38)
				points, _ := record.Uint32(39)
				flags, _ := record.Uint32(41)
				minimum, _ := record.Uint32(60)
				achievementIndex.achieveByID[id] = achievementEntry{ID: id, Faction: faction, Category: category, Points: points, Flags: flags, MinimumCriteria: minimum}
			}
		}
	}
	if s.WorldStore != nil && s.WorldStore.DB != nil {
		if rows, err := s.WorldStore.DB.Query("SELECT criteria_id, type, value1, value2, ScriptName FROM achievement_criteria_data"); err == nil {
			for rows.Next() {
				var cid, ctype, v1, v2 uint32
				var script string
				if rows.Scan(&cid, &ctype, &v1, &v2, &script) == nil {
					achievementIndex.criteriaData[cid] = append(achievementIndex.criteriaData[cid], criteriaDataEntry{
						Type:       ctype,
						Value1:     v1,
						Value2:     v2,
						ScriptName: script,
					})
				}
			}
			rows.Close()
		}
	}
	achievementIndex.loaded = true
}

// loadAchievementState reads character_achievement and progress rows for a
// player, mirroring AchievementMgr::LoadFromDB.
func (s *session) loadAchievementState(ctx context.Context) {
	s.earnedAchievements = make(map[uint32]uint32)
	s.criteriaProgress = make(map[uint32]*criteriaProgressState)
	s.server.loadAchievementIndex()
	cdb := s.server.CharactersStore
	if cdb == nil || cdb.DB == nil {
		return
	}
	achievementIndex.mu.RLock()
	validateAchievements := len(achievementIndex.achieveByID) != 0
	validateCriteria := len(achievementIndex.byID) != 0
	achievementIndex.mu.RUnlock()
	rows, err := cdb.DB.QueryContext(ctx, "SELECT achievement, date FROM character_achievement WHERE guid = ?", s.playerGUID)
	if err == nil {
		for rows.Next() {
			var id, date uint32
			if rows.Scan(&id, &date) != nil {
				continue
			}
			if validateAchievements {
				achievementIndex.mu.RLock()
				_, found := achievementIndex.achieveByID[id]
				achievementIndex.mu.RUnlock()
				if !found {
					s.debug("achievement load skipped", "guid", s.playerGUID, "achievement", id, "reason", "unknown achievement")
					continue
				}
			}
			s.earnedAchievements[id] = date
		}
		rows.Close()
	}
	rows, err = cdb.DB.QueryContext(ctx, "SELECT criteria, counter, date FROM character_achievement_progress WHERE guid = ?", s.playerGUID)
	if err == nil {
		for rows.Next() {
			var id, counter, date uint32
			if rows.Scan(&id, &counter, &date) != nil {
				continue
			}
			if validateCriteria {
				achievementIndex.mu.RLock()
				criteria, found := achievementIndex.byID[id]
				achievementIndex.mu.RUnlock()
				if !found {
					s.debug("achievement criteria load skipped", "guid", s.playerGUID, "criteria", id, "reason", "unknown criteria")
					continue
				}
				if criteria.StartTimer > 0 && int64(date)+int64(criteria.StartTimer) < time.Now().Unix() {
					continue
				}
			}
			s.criteriaProgress[id] = &criteriaProgressState{CriteriaID: id, Counter: counter, Date: date}
		}
		rows.Close()
	}
}

// writeEarnedAchievement appends one EarnedAchievement block (u32 id, u32 date).
func writeEarnedAchievement(buffer *protocol.Buffer, id, date uint32) {
	buffer.WriteU32(id)
	buffer.WritePackedTime(time.Unix(int64(date), 0))
}

// writeCriteriaProgress appends one CriteriaProgress block per
// AchievementPackets.cpp: criteria id, packed quantity, packed player GUID,
// flags, date, time from start, time from create.
func writeCriteriaProgress(buffer *protocol.Buffer, playerGUID uint64, progress *criteriaProgressState) {
	buffer.WriteU32(progress.CriteriaID)
	buffer.WritePackedGUID(uint64(progress.Counter))
	buffer.WritePackedGUID(playerGUID)
	buffer.WriteU32(0) // flags
	buffer.WritePackedTime(time.Unix(int64(progress.Date), 0))
	buffer.WriteU32(0) // elapsed time
	buffer.WriteU32(0) // creation time
}

func (s *session) hiddenAchievement(achievementID uint32) bool {
	if s == nil || s.server == nil {
		return false
	}
	s.server.loadAchievementIndex()
	achievementIndex.mu.RLock()
	entry, found := achievementIndex.achieveByID[achievementID]
	achievementIndex.mu.RUnlock()
	return found && entry.Flags&achievementFlagHidden != 0
}

// sendAllAchievementData mirrors AchievementMgr::SendAllAchievementData:
// earned block, -1 separator, progress block, -1 separator.
func (s *session) sendAllAchievementData() {
	if s.player == nil {
		return
	}
	packet := protocol.NewBuffer(256)
	ids := make([]uint32, 0, len(s.earnedAchievements))
	for id := range s.earnedAchievements {
		if s.hiddenAchievement(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		writeEarnedAchievement(packet, id, s.earnedAchievements[id])
	}
	packet.WriteU32(0xFFFFFFFF)
	progressIDs := make([]uint32, 0, len(s.criteriaProgress))
	for id := range s.criteriaProgress {
		progressIDs = append(progressIDs, id)
	}
	sort.Slice(progressIDs, func(i, j int) bool { return progressIDs[i] < progressIDs[j] })
	for _, id := range progressIDs {
		writeCriteriaProgress(packet, s.playerGUID, s.criteriaProgress[id])
	}
	packet.WriteU32(0xFFFFFFFF)
	_ = s.write(uint16(protocol.OpcodeSMSG_ALL_ACHIEVEMENT_DATA), packet.Bytes(), true)
}

// handleQueryInspectAchievements answers CMSG_QUERY_INSPECT_ACHIEVEMENTS with
// the target's real achievement state from the database, mirroring
// RespondInspectAchievements (packed target GUID, earned block, -1,
// progress block, -1).
func (s *session) handleQueryInspectAchievements(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	targetGUID, err := r.ReadU64()
	if err != nil {
		return false
	}
	cdb := s.server.CharactersStore
	if cdb == nil || cdb.DB == nil {
		return true
	}
	earned := make(map[uint32]uint32)
	progress := make(map[uint32]*criteriaProgressState)
	if rows, err := cdb.DB.QueryContext(ctx, "SELECT achievement, date FROM character_achievement WHERE guid = ?", targetGUID); err == nil {
		for rows.Next() {
			var id, date uint32
			if rows.Scan(&id, &date) == nil {
				earned[id] = date
			}
		}
		rows.Close()
	}
	if rows, err := cdb.DB.QueryContext(ctx, "SELECT criteria, counter, date FROM character_achievement_progress WHERE guid = ?", targetGUID); err == nil {
		for rows.Next() {
			var id, counter, date uint32
			if rows.Scan(&id, &counter, &date) == nil {
				progress[id] = &criteriaProgressState{CriteriaID: id, Counter: counter, Date: date}
			}
		}
		rows.Close()
	}
	packet := protocol.NewBuffer(64)
	packet.WritePackedGUID(targetGUID)
	ids := make([]uint32, 0, len(earned))
	for id := range earned {
		if s.hiddenAchievement(id) {
			continue
		}
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		writeEarnedAchievement(packet, id, earned[id])
	}
	packet.WriteU32(0xFFFFFFFF)
	progressIDs := make([]uint32, 0, len(progress))
	for id := range progress {
		progressIDs = append(progressIDs, id)
	}
	sort.Slice(progressIDs, func(i, j int) bool { return progressIDs[i] < progressIDs[j] })
	for _, id := range progressIDs {
		writeCriteriaProgress(packet, targetGUID, progress[id])
	}
	packet.WriteU32(0xFFFFFFFF)
	return s.write(uint16(protocol.OpcodeSMSG_RESPOND_INSPECT_ACHIEVEMENTS), packet.Bytes(), true) == nil
}

// sendCriteriaUpdate mirrors AchievementPackets CriteriaUpdate.
func (s *session) sendCriteriaUpdate(progress *criteriaProgressState) {
	packet := protocol.NewBuffer(32)
	packet.WriteU32(progress.CriteriaID)
	packet.WritePackedGUID(uint64(progress.Counter))
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(0) // flags
	packet.WritePackedTime(time.Unix(int64(progress.Date), 0))
	packet.WriteU32(0) // elapsed
	packet.WriteU32(0) // creation
	if s.playerLoading {
		return
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_CRITERIA_UPDATE), packet.Bytes(), true)
}

// playerHasTitle returns true if the player has the given title ID chosen or known.
func (s *session) playerHasTitle(titleID uint32) bool {
	if s.player == nil {
		return false
	}
	if s.player.ChosenTitle == titleID {
		return true
	}
	idx := titleID / 32
	bit := uint32(1) << (titleID % 32)
	if int(idx) < len(s.player.KnownTitles) && (s.player.KnownTitles[idx]&bit) != 0 {
		return true
	}
	return false
}

// playerHasAura returns true if the player has an active aura from spellID.
func (s *session) playerHasAura(spellID uint32) bool {
	if s.activeAuras == nil {
		return false
	}
	aura, ok := s.activeAuras[spellID]
	return ok && aura != nil && !aura.Stopped
}

// meetsCriteriaRequirements checks DBC AdditionalRequirements and
// achievement_criteria_data database rules before awarding criteria progress,
// mirroring AchievementMgr::ConditionsSatisfied and AchievementCriteriaDataSet::Meets.
func (s *session) meetsCriteriaRequirements(criterion achievementCriteriaEntry, miscValue1, miscValue2 uint32) bool {
	if s.player == nil {
		return false
	}

	achievementIndex.mu.RLock()
	achieve, hasAchieve := achievementIndex.achieveByID[criterion.AchievementID]
	rules := achievementIndex.criteriaData[criterion.ID]
	achievementIndex.mu.RUnlock()

	// Faction check
	if hasAchieve && achieve.Faction >= 0 {
		team := playerTeam(s.player.Race)
		if (achieve.Faction == 0 && team != teamHorde) || (achieve.Faction == 1 && team != teamAlliance) {
			return false
		}
	}

	// DBC AdditionalRequirements checks
	reqs := [2]struct {
		typ   uint32
		asset uint32
	}{
		{criterion.ReqType1, criterion.ReqAsset1},
		{criterion.ReqType2, criterion.ReqAsset2},
	}
	for _, req := range reqs {
		if req.typ == 0 {
			continue
		}
		switch req.typ {
		case criteriaConditionBGMap: // 3: requires player to be on specific map
			if s.player.Map != req.asset {
				return false
			}
		case criteriaConditionNotInGroup: // 10: requires player not to be in group
			if s.groupID != 0 {
				return false
			}
		}
	}

	// achievement_criteria_data DB rules
	for _, rule := range rules {
		switch rule.Type {
		case criteriaDataTypeMapID: // 20
			if s.player.Map != rule.Value1 {
				return false
			}
		case criteriaDataTypeSArea: // 6
			if s.player.Zone != rule.Value1 {
				return false
			}
		case criteriaDataTypeSPlayerClassRace: // 21
			if rule.Value1 != 0 && uint32(s.player.Class) != rule.Value1 {
				return false
			}
			if rule.Value2 != 0 && uint32(s.player.Race) != rule.Value2 {
				return false
			}
		case criteriaDataTypeSKnownTitle: // 23
			if !s.playerHasTitle(rule.Value1) {
				return false
			}
		case criteriaDataTypeSAura: // 5
			if !s.playerHasAura(rule.Value1) {
				return false
			}
		case criteriaDataTypeMapDifficulty: // 12
			diff := uint32(s.player.DungeonDifficulty)
			if s.player.RaidDifficulty != 0 {
				diff = uint32(s.player.RaidDifficulty)
			}
			if diff < rule.Value1 {
				return false
			}
		case criteriaDataTypeValue: // 8
			if miscValue1 < rule.Value1 {
				return false
			}
		case criteriaDataTypeSDrunk: // 15
			if uint32(s.player.DrunkenState) < rule.Value1 {
				return false
			}
		case criteriaDataTypeSItemQuality: // 25
			if miscValue1 != rule.Value1 {
				return false
			}
		}
	}

	return true
}

// updateAchievementCriteria advances every criterion matching the type and
// asset by quantity, mirroring AchievementMgr::UpdateAchievementCriteria for
// the counter-style criteria this server tracks.
func (s *session) updateAchievementCriteria(criterionType, asset uint32, quantity uint32) {
	if s.player == nil || s.server == nil {
		return
	}
	if s.earnedAchievements == nil {
		s.earnedAchievements = make(map[uint32]uint32)
	}
	if s.criteriaProgress == nil {
		s.criteriaProgress = make(map[uint32]*criteriaProgressState)
	}
	s.server.loadAchievementIndex()
	achievementIndex.mu.RLock()
	var matched []achievementCriteriaEntry
	if list, ok := achievementIndex.byTypeAsset[typeAssetKey(criterionType, asset)]; ok {
		matched = append(matched, list...)
	}
	if asset != 0 {
		if list0, ok := achievementIndex.byTypeAsset[typeAssetKey(criterionType, 0)]; ok {
			matched = append(matched, list0...)
		}
	}
	achievementIndex.mu.RUnlock()
	if len(matched) == 0 {
		return
	}

	for _, criterion := range matched {
		if _, done := s.earnedAchievements[criterion.AchievementID]; done {
			continue
		}
		if !s.meetsCriteriaRequirements(criterion, quantity, asset) {
			continue
		}
		progress := s.criteriaProgress[criterion.ID]
		if progress == nil {
			progress = &criteriaProgressState{CriteriaID: criterion.ID}
			s.criteriaProgress[criterion.ID] = progress
		}
		progress.Counter += quantity
		progress.Date = uint32(time.Now().Unix())
		s.sendCriteriaUpdate(progress)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(),
				"REPLACE INTO character_achievement_progress (guid, criteria, counter, date) VALUES (?, ?, ?, ?)",
				s.playerGUID, criterion.ID, progress.Counter, progress.Date)
		}
		if criterion.Quantity > 0 && progress.Counter >= criterion.Quantity {
			s.stopTimedAchievement(criterion.ID)
			s.checkAchievementComplete(criterion.AchievementID)
		}
	}
}

// setAchievementCriteria sets an absolute criteria value (level, skill
// value, reputation standing) mirroring the reference PROGRESS_SET updates.
func (s *session) setAchievementCriteria(criterionType, asset, value uint32) {
	if s.player == nil || s.server == nil {
		return
	}
	if s.earnedAchievements == nil {
		s.earnedAchievements = make(map[uint32]uint32)
	}
	if s.criteriaProgress == nil {
		s.criteriaProgress = make(map[uint32]*criteriaProgressState)
	}
	s.server.loadAchievementIndex()
	achievementIndex.mu.RLock()
	var matched []achievementCriteriaEntry
	if list, ok := achievementIndex.byTypeAsset[typeAssetKey(criterionType, asset)]; ok {
		matched = append(matched, list...)
	}
	if asset != 0 {
		if list0, ok := achievementIndex.byTypeAsset[typeAssetKey(criterionType, 0)]; ok {
			matched = append(matched, list0...)
		}
	}
	achievementIndex.mu.RUnlock()
	if len(matched) == 0 {
		return
	}

	for _, criterion := range matched {
		if _, done := s.earnedAchievements[criterion.AchievementID]; done {
			continue
		}
		if !s.meetsCriteriaRequirements(criterion, value, asset) {
			continue
		}
		progress := s.criteriaProgress[criterion.ID]
		if progress == nil {
			progress = &criteriaProgressState{CriteriaID: criterion.ID}
			s.criteriaProgress[criterion.ID] = progress
		}
		if progress.Counter >= value {
			continue // absolute values never regress
		}
		progress.Counter = value
		progress.Date = uint32(time.Now().Unix())
		s.sendCriteriaUpdate(progress)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(),
				"REPLACE INTO character_achievement_progress (guid, criteria, counter, date) VALUES (?, ?, ?, ?)",
				s.playerGUID, criterion.ID, progress.Counter, progress.Date)
		}
		if criterion.Quantity > 0 && progress.Counter >= criterion.Quantity {
			s.checkAchievementComplete(criterion.AchievementID)
		}
	}
}

// Timed criteria engine, mirroring AchievementMgr::StartTimedAchievement,
// UpdateTimedAchievements, and RemoveTimedAchievement (AchievementMgr.cpp:1461).
// Timed types from DBCEnums.h: 1 event, 2 quest accept, 5 spell cast,
// 6 spell target, 7 creature kill, 9 item use. Starting arms a StartTimer-
// second deadline; expiry resets the criteria progress, notifies the client
// with SMSG_CRITERIA_DELETED, and removes the persisted row.
const (
	timedTypeQuest       = 2
	timedTypeSpellCast   = 5
	timedTypeSpellTarget = 6
	timedTypeCreature    = 7
	timedTypeItem        = 9
)

// startTimedAchievement mirrors AchievementMgr::StartTimedAchievement: for
// every criteria whose StartEvent matches the timed type and StartAsset
// matches the entry, reset progress to zero and arm the deadline.
func (s *session) startTimedAchievement(timedType, entry uint32) {
	if s.player == nil || s.server == nil {
		return
	}
	s.server.loadAchievementIndex()
	achievementIndex.mu.RLock()
	list, ok := achievementIndex.byTimedEvent[typeAssetKey(timedType, entry)]
	if !ok {
		achievementIndex.mu.RUnlock()
		return
	}
	matched := make([]achievementCriteriaEntry, len(list))
	copy(matched, list)
	achievementIndex.mu.RUnlock()

	if s.earnedAchievements == nil {
		s.earnedAchievements = make(map[uint32]uint32)
	}
	if s.criteriaProgress == nil {
		s.criteriaProgress = make(map[uint32]*criteriaProgressState)
	}
	if s.timedCriteria == nil {
		s.timedCriteria = make(map[uint32]*time.Timer)
	}
	for _, criterion := range matched {
		if _, done := s.earnedAchievements[criterion.AchievementID]; done {
			continue
		}
		if _, running := s.timedCriteria[criterion.ID]; running {
			continue
		}
		if criterion.StartTimer == 0 {
			continue
		}
		progress := &criteriaProgressState{CriteriaID: criterion.ID, Date: uint32(time.Now().Unix())}
		s.criteriaProgress[criterion.ID] = progress
		s.sendCriteriaUpdate(progress)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(),
				"REPLACE INTO character_achievement_progress (guid, criteria, counter, date) VALUES (?, ?, 0, ?)",
				s.playerGUID, criterion.ID, progress.Date)
		}
		timer := time.AfterFunc(time.Duration(criterion.StartTimer)*time.Second, func() {
			s.expireTimedAchievement(criterion.ID)
		})
		s.timedCriteria[criterion.ID] = timer
		s.debug("timed achievement started", "account", s.accountName, "criteria", criterion.ID, "seconds", criterion.StartTimer)
	}
}

// removeCriteriaProgress removes progress for a single criterion, cancels any
// active timer, notifies the client with SMSG_CRITERIA_DELETED, and deletes persistence,
// mirroring AchievementMgr::RemoveCriteriaProgress.
func (s *session) removeCriteriaProgress(criteriaID uint32) {
	if s.criteriaProgress == nil {
		return
	}
	if _, has := s.criteriaProgress[criteriaID]; !has {
		return
	}
	delete(s.criteriaProgress, criteriaID)
	if s.timedCriteria != nil {
		if timer, has := s.timedCriteria[criteriaID]; has {
			timer.Stop()
			delete(s.timedCriteria, criteriaID)
		}
	}
	packet := protocol.NewBuffer(4)
	packet.WriteU32(criteriaID)
	_ = s.write(uint16(protocol.OpcodeSMSG_CRITERIA_DELETED), packet.Bytes(), true)
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(),
			"DELETE FROM character_achievement_progress WHERE guid = ? AND criteria = ?",
			s.playerGUID, criteriaID)
	}
}

// expireTimedAchievement mirrors the UpdateTimedAchievements expiry path.
func (s *session) expireTimedAchievement(criteriaID uint32) {
	s.removeCriteriaProgress(criteriaID)
	s.debug("timed achievement expired", "account", s.accountName, "criteria", criteriaID)
}

func (s *session) stopTimedAchievements() {
	if s == nil {
		return
	}
	for criteriaID, timer := range s.timedCriteria {
		if timer != nil {
			timer.Stop()
		}
		delete(s.timedCriteria, criteriaID)
	}
}

// resetAchievementCriteriaByCondition resets progress for unearned achievements
// whose criteria match the condition (e.g. no-death on death, no-lose on arena loss),
// mirroring AchievementMgr::ResetAchievementCriteria.
func (s *session) resetAchievementCriteriaByCondition(condition, value uint32) {
	if s.player == nil || s.server == nil {
		return
	}
	s.server.loadAchievementIndex()
	achievementIndex.mu.RLock()
	criterias := make([]achievementCriteriaEntry, len(achievementIndex.byCondition[condition]))
	copy(criterias, achievementIndex.byCondition[condition])
	achievementIndex.mu.RUnlock()

	for _, criterion := range criterias {
		if value != 0 {
			if (criterion.ReqType1 == condition && criterion.ReqAsset1 != value) ||
				(criterion.ReqType2 == condition && criterion.ReqAsset2 != value) {
				continue
			}
		}
		if _, done := s.earnedAchievements[criterion.AchievementID]; done {
			continue
		}
		if progress, ok := s.criteriaProgress[criterion.ID]; ok {
			if criterion.Quantity > 0 && progress.Counter >= criterion.Quantity {
				continue
			}
			s.removeCriteriaProgress(criterion.ID)
		}
	}
}

// stopTimedAchievement mirrors RemoveTimedAchievement without the deletion
// notification: used when the criteria completes inside the window.
func (s *session) stopTimedAchievement(criteriaID uint32) {
	if s.timedCriteria == nil {
		return
	}
	if timer, has := s.timedCriteria[criteriaID]; has {
		timer.Stop()
		delete(s.timedCriteria, criteriaID)
	}
}

// checkAchievementComplete completes the achievement when every tracked
// criterion of it has met its quantity, then announces and persists it.
func (s *session) checkAchievementComplete(achievementID uint32) {
	achievementIndex.mu.RLock()
	criteria := achievementIndex.byAchieve[achievementID]
	entry, hasEntry := achievementIndex.achieveByID[achievementID]
	achievementIndex.mu.RUnlock()
	if len(criteria) == 0 || !hasEntry {
		return
	}
	if entry.Faction >= 0 {
		team := playerTeam(s.player.Race)
		if (entry.Faction == 0 && team != teamHorde) || (entry.Faction == 1 && team != teamAlliance) {
			return
		}
	}
	completedCount := uint32(0)
	for _, criterion := range criteria {
		progress := s.criteriaProgress[criterion.ID]
		if progress != nil && criterion.Quantity > 0 && progress.Counter >= criterion.Quantity {
			completedCount++
		}
	}
	needed := entry.MinimumCriteria
	if needed == 0 {
		needed = uint32(len(criteria))
	}
	if completedCount < needed {
		return
	}
	s.completeAchievement(achievementID)
}

// completeAchievement mirrors AchievementMgr::CompletedAchievement:
// SMSG_ACHIEVEMENT_EARNED (packed earner, id, time, initial=1) to the player
// and nearby players, persisted to character_achievement.
func (s *session) completeAchievement(achievementID uint32) {
	if _, done := s.earnedAchievements[achievementID]; done {
		return
	}
	now := uint32(time.Now().Unix())
	s.earnedAchievements[achievementID] = now
	if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(context.Background(),
			"INSERT OR IGNORE INTO character_achievement (guid, achievement, date) VALUES (?, ?, ?)",
			s.playerGUID, achievementID, now)
	}
	packet := protocol.NewBuffer(24)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(achievementID)
	packet.WritePackedTime(time.Unix(int64(now), 0))
	packet.WriteU32(0)
	if !s.playerLoading && !s.hiddenAchievement(achievementID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_ACHIEVEMENT_EARNED), packet.Bytes(), true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ACHIEVEMENT_EARNED), packet.Bytes(), s)
		}
	}
	s.debug("achievement earned", "account", s.accountName, "guid", s.playerGUID, "achievement", achievementID)

	// Reference COMPLETE_ACHIEVEMENT: completing an achievement advances meta-achievements
	s.updateAchievementCriteria(criteriaTypeCompleteAchievement, achievementID, 1)

	// Reference EARN_ACHIEVEMENT_POINTS: sum up points from all earned achievements
	totalPoints := uint32(0)
	achievementIndex.mu.RLock()
	for earnedID := range s.earnedAchievements {
		if entry, ok := achievementIndex.achieveByID[earnedID]; ok {
			totalPoints += entry.Points
		}
	}
	achievementIndex.mu.RUnlock()
	s.setAchievementCriteria(criteriaTypeEarnAchievementPoints, 0, totalPoints)
}

// achievementCriteriaCount is used by tests to inspect the index size.
func achievementCriteriaCount() int {
	achievementIndex.mu.RLock()
	defer achievementIndex.mu.RUnlock()
	return len(achievementIndex.byID)
}

// creditBGObjectiveCapture mirrors the reference BG_OBJECTIVE_CAPTURE
// achievement credit (AchievementMgr::UpdateAchievementCriteria with
// ACHIEVEMENT_CRITERIA_TYPE_BG_OBJECTIVE_CAPTURE): the player whose assault
// completed the objective gains progress for that objective id.
func (s *Server) creditBGObjectiveCapture(playerGUID uint64, objectiveID uint32) {
	if playerGUID == 0 {
		return
	}
	sess := s.findSessionByGUID(playerGUID)
	if sess == nil || sess.player == nil {
		return
	}
	sess.updateAchievementCriteria(criteriaTypeBGObjective, objectiveID, 1)
}

// creditHonorableKill mirrors the reference honorable-kill achievement chain
// (Unit::Kill -> UpdateAchievementCriteria HONORABLE_KILL, HK_CLASS, HK_RACE):
// the killer gains one honorable kill plus per-class and per-race credit for
// the victim. Duel kills are excluded, matching the reference honor rules.
func (s *Server) creditHonorableKill(killer, victim *session) {
	if killer == nil || victim == nil || killer == victim {
		return
	}
	if killer.player == nil || victim.player == nil {
		return
	}
	if killer.duelPartner != 0 && killer.duelPartner == victim.playerGUID {
		return // duels are not honorable kills
	}
	if victim.player.Race == 0 || victim.player.Class == 0 {
		return
	}
	if killer.player.TodayKills < ^uint16(0) {
		killer.player.TodayKills++
	}
	killer.player.TotalKills++
	if killer.server != nil && killer.server.CharactersStore != nil && killer.server.CharactersStore.DB != nil {
		_, _ = killer.server.CharactersStore.DB.Exec("UPDATE characters SET totalKills = ?, todayKills = ? WHERE guid = ?", killer.player.TotalKills, killer.player.TodayKills, killer.playerGUID)
	}
	killer.sendPlayerUpdate()
	killer.updateAchievementCriteria(criteriaTypeHonorableKill, 0, 1)
	killer.updateAchievementCriteria(criteriaTypeEarnHonorableKill, 0, 1)
	if killer.player.Zone > 0 {
		killer.updateAchievementCriteria(criteriaTypeHKAtArea, killer.player.Zone, 1)
	}
	victim.updateAchievementCriteria(criteriaTypeKilledByPlayer, 0, 1)
	killer.updateAchievementCriteria(criteriaTypeHKClass, uint32(victim.player.Class), 1)
	killer.updateAchievementCriteria(criteriaTypeHKRace, uint32(victim.player.Race), 1)
	killer.updateAchievementCriteria(criteriaTypeSpecialPvPKill, 0, 1)
	killer.creditPlayerKillQuest(context.Background())
}

// exploreZone mirrors Player::UpdateZone exploration (Player.cpp:6565): set
// the AreaTable AreaBit in the PLAYER_EXPLORED_ZONES bitfield (persisted to
// characters.exploredZones), push the changed field to the client, and
// complete every EXPLORE_AREA criteria whose WorldMapOverlay covers the zone
// (AchievementMgr.cpp:1881 match semantics).
func (s *session) exploreZone(ctx context.Context, zoneID uint32) {
	if s.player == nil || s.server == nil || s.server.Data == nil || zoneID == 0 {
		return
	}
	areaBit, _, found, err := s.server.Data.AreaTableInfo(zoneID)
	if err != nil || !found || areaBit < 0 {
		return
	}
	bit := uint32(areaBit)
	offset := bit / 32
	if offset >= playerExploredZonesCount {
		return
	}
	mask := uint32(1) << (bit % 32)
	if s.player.ExploredZones[offset]&mask != 0 {
		return // already explored
	}
	s.player.ExploredZones[offset] |= mask
	s.persistExploredZones(ctx)

	// Push the changed explored-zones field to the client.
	s.server.loadAchievementIndex()
	achievementIndex.mu.RLock()
	criteriaIDs := make([]uint32, len(achievementIndex.exploreByZone[zoneID]))
	copy(criteriaIDs, achievementIndex.exploreByZone[zoneID])
	achievementIndex.mu.RUnlock()

	if s.earnedAchievements == nil {
		s.earnedAchievements = make(map[uint32]uint32)
	}
	if s.criteriaProgress == nil {
		s.criteriaProgress = make(map[uint32]*criteriaProgressState)
	}
	for _, criteriaID := range criteriaIDs {
		achievementIndex.mu.RLock()
		criterion, has := achievementIndex.byID[criteriaID]
		achievementIndex.mu.RUnlock()
		if !has {
			continue
		}
		if _, done := s.earnedAchievements[criterion.AchievementID]; done {
			continue
		}
		progress := s.criteriaProgress[criteriaID]
		if progress == nil {
			progress = &criteriaProgressState{CriteriaID: criteriaID}
			s.criteriaProgress[criteriaID] = progress
		}
		if progress.Counter >= 1 {
			continue
		}
		progress.Counter = 1
		progress.Date = uint32(time.Now().Unix())
		s.sendCriteriaUpdate(progress)
		if s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
				"REPLACE INTO character_achievement_progress (guid, criteria, counter, date) VALUES (?, ?, 1, ?)",
				s.playerGUID, criteriaID, progress.Date)
		}
		s.stopTimedAchievement(criteriaID)
		s.checkAchievementComplete(criterion.AchievementID)
	}
	s.debug("zone explored", "account", s.accountName, "zone", zoneID, "criteria", len(criteriaIDs))
}

// persistExploredZones writes the explored bitfield as hex to characters.
func (s *session) persistExploredZones(ctx context.Context) {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	blob := make([]byte, playerExploredZonesCount*4)
	for i, value := range s.player.ExploredZones {
		blob[i*4] = byte(value)
		blob[i*4+1] = byte(value >> 8)
		blob[i*4+2] = byte(value >> 16)
		blob[i*4+3] = byte(value >> 24)
	}
	const hexDigits = "0123456789abcdef"
	hex := make([]byte, len(blob)*2)
	for i, b := range blob {
		hex[i*2] = hexDigits[b>>4]
		hex[i*2+1] = hexDigits[b&0x0F]
	}
	_, _ = s.server.CharactersStore.DB.ExecContext(ctx,
		"UPDATE characters SET exploredZones = ? WHERE guid = ?", string(hex), s.playerGUID)
}

// loadExploredZones reads the hex blob back into the bitfield at login.
func (s *session) loadExploredZones(ctx context.Context) {
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || s.player == nil {
		return
	}
	var hex string
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
		"SELECT COALESCE(exploredZones, '') FROM characters WHERE guid = ?", s.playerGUID).Scan(&hex); err != nil {
		return
	}
	if len(hex) == 0 {
		return
	}
	for i := 0; i < playerExploredZonesCount && (i*2+1) < len(hex); i++ {
		hi := hexDigitValue(hex[i*2])
		lo := hexDigitValue(hex[i*2+1])
		if hi < 0 || lo < 0 {
			return // corrupt blob: keep zero state
		}
		s.player.ExploredZones[i] = uint32(hi)<<4 | uint32(lo)
	}
}

func hexDigitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// creditBattlegroundWin mirrors the reference WIN_BG criteria credit at
// battleground end: every online player of the winning team on the
// battleground map gains progress, and all participants complete the battleground.
func (s *Server) creditBattlegroundWin(mapID, winningTeam uint32) {
	s.sessionsMu.RLock()
	var allParticipants, winners []*session
	for sess := range s.sessions {
		if !sess.worldReady.Load() || sess.player == nil || sess.player.Map != mapID {
			continue
		}
		allParticipants = append(allParticipants, sess)
		if teamForRace(sess.player.Race) == winningTeam {
			winners = append(winners, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range allParticipants {
		sess.updateAchievementCriteria(criteriaTypeCompleteBattleground, mapID, 1)
	}
	for _, sess := range winners {
		sess.updateAchievementCriteria(criteriaTypeWinBG, mapID, 1)
	}
}

// creditArenaParticipants mirrors PLAY_ARENA / WIN_ARENA / WIN_RATED_ARENA / GET_KILLING_BLOWS
// at arena end: every player on the arena map gains play credit, winners gain
// the win, and killing blow totals come from the scoreboard.
func (s *Server) creditArenaParticipants(mapID uint32, scores map[uint64]uint32, winners map[uint64]struct{}) {
	s.sessionsMu.RLock()
	var participants []*session
	for sess := range s.sessions {
		if sess.worldReady.Load() && sess.player != nil && sess.player.Map == mapID {
			participants = append(participants, sess)
		}
	}
	s.sessionsMu.RUnlock()
	for _, sess := range participants {
		sess.updateAchievementCriteria(criteriaTypePlayArena, mapID, 1)
		if blows, has := scores[sess.playerGUID]; has && blows > 0 {
			sess.updateAchievementCriteria(criteriaTypeGetKillingBlows, mapID, blows)
		}
		if _, won := winners[sess.playerGUID]; won {
			sess.updateAchievementCriteria(criteriaTypeWinArena, mapID, 1)
			sess.updateAchievementCriteria(criteriaTypeWinRatedArena, 0, 1)
		} else {
			sess.resetAchievementCriteriaByCondition(criteriaConditionNoLose, 0)
		}
	}
}
