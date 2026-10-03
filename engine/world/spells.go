package world

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/data/wotlk"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/scripting"
	"github.com/MorenoLand/Moreno.Go-MorenoCore/pkg/protocol"
)

const (
	spellAttributePassive uint32 = 0x00000040
	spellCastFlagStart    uint32 = 0x00000002
	spellCastFlagGo       uint32 = 0x00000100
	spellCastFlagPending  uint32 = 0x00000001

	spellAttr3MainHand             uint32 = 0x00000400 // SPELL_ATTR3_MAIN_HAND: Require main hand weapon (SharedDefines.h:533)
	spellAttr3ReqOffhand           uint32 = 0x01000000 // SPELL_ATTR3_REQ_OFFHAND: Require offhand weapon (SharedDefines.h:547)
	spellAttr3ReqWand              uint32 = 0x00400000 // SPELL_ATTR3_REQ_WAND: Requires equipped Wand (SharedDefines.h:545)
	spellAttr5HideDuration         uint32 = 0x00000400 // SPELL_ATTR5_HIDE_DURATION (SharedDefines.h:607)
	spellAttr5CanChannelWhenMoving uint32 = 0x00000001 // SPELL_ATTR5_CAN_CHANNEL_WHEN_MOVING (SharedDefines.h:597)
	spellAttr5SingleTarget         uint32 = 0x00000020 // SPELL_ATTR5_SINGLE_TARGET_SPELL (SharedDefines.h:602)

	spellInterruptFlagMovement uint32 = 0x01 // SPELL_INTERRUPT_FLAG_MOVEMENT (SpellDefines.h:30)

	spellAttr1NotBreakStealth  uint32 = 0x00000020 // SPELL_ATTR1_NOT_BREAK_STEALTH (SharedDefines.h:454)
	spellAttr1NoThreat         uint32 = 0x00000400 // SPELL_ATTR1_NO_THREAT (SharedDefines.h:459) — ATTR1 is Go's AttributesEx (Spell.dbc field 5)
	spellAttr1DismissPet       uint32 = 0x00000001 // SPELL_ATTR1_DISMISS_PET (SharedDefines.h:449) — ATTR1 is Go's AttributesEx (Spell.dbc field 5)
	spellAttr1MeleeCombatStart uint32 = 0x00000200 // SPELL_ATTR1_MELEE_COMBAT_START (SharedDefines.h:458) — caster begins auto-attack on cast
	spellAttr3NoInitialAggro   uint32 = 0x00020000 // SPELL_ATTR3_NO_INITIAL_AGGRO (SharedDefines.h:540) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7)

	spellAttr0Ability                      uint32 = 0x00000010 // SPELL_ATTR0_ABILITY (SharedDefines.h:416)
	spellAttr0ReqAmmo                      uint32 = 0x00000002 // SPELL_ATTR0_REQ_AMMO (SharedDefines.h:413)
	spellAttr0Tradespell                   uint32 = 0x00000020 // SPELL_ATTR0_TRADESPELL (SharedDefines.h:417)
	spellAttr3NoDoneBonus                  uint32 = 0x20000000 // SPELL_ATTR3_NO_DONE_BONUS (SharedDefines.h:552) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)
	spellAttr3TreatAsPeriodic              uint32 = 0x02000000 // SPELL_ATTR3_TREAT_AS_PERIODIC (SharedDefines.h:548) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)
	spellAttr3StackForDiffCasters          uint32 = 0x00000080 // SPELL_ATTR3_STACK_FOR_DIFF_CASTERS (SharedDefines.h:530) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)
	spellAttr7NoPushbackOnDamage           uint32 = 0x00000040 // SPELL_ATTR7_NO_PUSHBACK_ON_DAMAGE (SharedDefines.h:677) — ATTR7 is Go's AttributesEx7 (Spell.dbc field 11 = AttributesExG)
	spellAttr7DispelCharges                uint32 = 0x00000400 // SPELL_ATTR7_DISPEL_CHARGES (SharedDefines.h:681) — ATTR7 is Go's AttributesEx7 (Spell.dbc field 11 = AttributesExG)
	spellAttr6AssistIgnoreImmuneFlag       uint32 = 0x00000008 // SPELL_ATTR6_ASSIST_IGNORE_IMMUNE_FLAG (SharedDefines.h:637) — ATTR6 is Go's AttributesEx6 (Spell.dbc field 10 = AttributesExF)
	spellAttr6CanTargetUntargetable        uint32 = 0x01000000 // SPELL_ATTR6_CAN_TARGET_UNTARGETABLE (SharedDefines.h:658) — ATTR6 is Go's AttributesEx6 (Spell.dbc field 10 = AttributesExF)
	spellAttr6DontConsumeProcCharges       uint32 = 0x00000020 // SPELL_ATTR6_DONT_CONSUME_PROC_CHARGES (SharedDefines.h:639) — ATTR6 is Go's AttributesEx6 (Spell.dbc field 10 = AttributesExF)
	spellAttr6NotInRaidInstance            uint32 = 0x00000800 // SPELL_ATTR6_NOT_IN_RAID_INSTANCE (SharedDefines.h:645) — ATTR6 is Go's AttributesEx6 (Spell.dbc field 10 = AttributesExF)
	spellAttr4NotStealable                 uint32 = 0x00000040 // SPELL_ATTR4_NOT_STEALABLE (SharedDefines.h:566) — ATTR4 is Go's AttributesEx4 (Spell.dbc field 8 = AttributesExD)
	spellAttr4FixedDamage                  uint32 = 0x00000100 // SPELL_ATTR4_FIXED_DAMAGE (SharedDefines.h:568) — ATTR4 is Go's AttributesEx4 (Spell.dbc field 8 = AttributesExD)
	spellAttr4NotUsableInArena             uint32 = 0x00010000 // SPELL_ATTR4_NOT_USABLE_IN_ARENA (SharedDefines.h:576) — ATTR4 is Go's AttributesEx4 (Spell.dbc field 8 = AttributesExD)
	spellAttr4UsableInArena                uint32 = 0x00020000 // SPELL_ATTR4_USABLE_IN_ARENA (SharedDefines.h:577) — ATTR4 is Go's AttributesEx4 (Spell.dbc field 8 = AttributesExD)
	spellAttr3Battleground                 uint32 = 0x00000800 // SPELL_ATTR3_BATTLEGROUND (SharedDefines.h:534) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)
	spellAttr4TreatAsDelayed               uint32 = 0x00000010 // SPELL_ATTR4_UNK4 "Treat as delayed spell" (SharedDefines.h:564) — ATTR4 is Go's AttributesEx4 (Spell.dbc field 8 = AttributesExD)
	spellAttr4ProcOnlyOnCaster             uint32 = 0x00000002 // SPELL_ATTR4_PROC_ONLY_ON_CASTER (SharedDefines.h:561) "Only proc on self-cast" — ATTR4 is Go's AttributesEx4
	spellAttr4CastOnlyInOutland            uint32 = 0x04000000 // SPELL_ATTR4_CAST_ONLY_IN_OUTLAND (SharedDefines.h:586) — ATTR4 is Go's AttributesEx4 (Spell.dbc field 8 = AttributesExD)
	targetUnitCaster                       uint32 = 1          // TARGET_UNIT_CASTER (SharedDefines.h:1442)
	spellAttr0UnaffectedByInvulnerability  uint32 = 0x20000000 // SPELL_ATTR0_UNAFFECTED_BY_INVULNERABILITY (SharedDefines.h:441)
	spellAttr0NotShapeshift                uint32 = 0x00010000 // SPELL_ATTR0_NOT_SHAPESHIFT (SharedDefines.h:428)
	spellAttr2NotNeedShapeshift            uint32 = 0x00080000 // SPELL_ATTR2_NOT_NEED_SHAPESHIFT (SharedDefines.h:505) — ATTR2 is Go's AttributesEx1 (Spell.dbc field 6 = AttributesExB)
	spellAttr1CantBeReflected              uint32 = 0x00000080 // SPELL_ATTR1_CANT_BE_REFLECTED (SharedDefines.h:456)
	spellAttr1ReqComboPoints1              uint32 = 0x00100000 // SPELL_ATTR1_REQ_COMBO_POINTS1 (SharedDefines.h:469) — ATTR1 is Go's AttributesEx (Spell.dbc field 5)
	spellAttr1ReqComboPoints2              uint32 = 0x00400000 // SPELL_ATTR1_REQ_COMBO_POINTS2 (SharedDefines.h:471) — ATTR1 is Go's AttributesEx (Spell.dbc field 5)
	spellAttr2CanTargetDead                uint32 = 0x00000001 // SPELL_ATTR2_CAN_TARGET_DEAD (SharedDefines.h:486) — ATTR2 is Go's AttributesEx1 (Spell.dbc field 6 = AttributesExB)
	spellAttr2AutorepeatFlag               uint32 = 0x00000020 // SPELL_ATTR2_AUTOREPEAT_FLAG (SharedDefines.h:491) — ATTR2 is Go's AttributesEx1 (Spell.dbc field 6 = AttributesExB)
	spellAttr2NotResetAutoActions          uint32 = 0x00020000 // SPELL_ATTR2_NOT_RESET_AUTO_ACTIONS (SharedDefines.h:503) — ATTR2 is Go's AttributesEx1 (Spell.dbc field 6 = AttributesExB)
	spellAttr5UsableWhileStunned           uint32 = 0x00000008 // SPELL_ATTR5_USABLE_WHILE_STUNNED (SharedDefines.h:600) — ATTR5 is Go's AttributesEx5 (Spell.dbc field 9 = AttributesExE)
	spellAttr5UsableWhileFeared            uint32 = 0x00020000 // SPELL_ATTR5_USABLE_WHILE_FEARED (SharedDefines.h:614) — ATTR5 is Go's AttributesEx5
	spellAttr5UsableWhileConfused          uint32 = 0x00040000 // SPELL_ATTR5_USABLE_WHILE_CONFUSED (SharedDefines.h:615) — ATTR5 is Go's AttributesEx5
	spellAttr5NoReagentWhilePrep           uint32 = 0x00000002 // SPELL_ATTR5_NO_REAGENT_WHILE_PREP (SharedDefines.h:598) — ATTR5 is Go's AttributesEx5
	spellAttr6IgnoreCasterAuras            uint32 = 0x00000004 // SPELL_ATTR6_IGNORE_CASTER_AURAS (SharedDefines.h:636) — ATTR6 is Go's AttributesEx6 (Spell.dbc field 10 = AttributesExF)
	spellAttr1DispelAurasOnImmunity        uint32 = 0x00008000 // SPELL_ATTR1_DISPEL_AURAS_ON_IMMUNITY (SharedDefines.h:464) — ATTR1 is Go's AttributesEx (Spell.dbc field 5)
	spellAttr2UnaffectedByAuraSchoolImmune uint32 = 0x04000000 // SPELL_ATTR2_UNAFFECTED_BY_AURA_SCHOOL_IMMUNE (SharedDefines.h:512) — ATTR2 is Go's AttributesEx1

	targetFlagCorpseEnemy uint32 = 0x00000200 // TARGET_FLAG_CORPSE_ENEMY (SpellInfo.h:57)
	targetFlagUnitDead    uint32 = 0x00000400 // TARGET_FLAG_UNIT_DEAD (SpellInfo.h:58)
	targetFlagCorpseAlly  uint32 = 0x00008000 // TARGET_FLAG_CORPSE_ALLY (SpellInfo.h:63)

	spellDamageClassMagic uint32 = 1 // SPELL_DAMAGE_CLASS_MAGIC (SharedDefines.h:1580)

	spellAttr0StopAttackTarget       uint32 = 0x00100000 // SPELL_ATTR0_STOP_ATTACK_TARGET (SharedDefines.h:432)
	spellAttr0DisabledWhileActive    uint32 = 0x02000000 // SPELL_ATTR0_DISABLED_WHILE_ACTIVE (SharedDefines.h:437)
	spellAttr0CastableWhileMounted   uint32 = 0x01000000 // SPELL_ATTR0_CASTABLE_WHILE_MOUNTED (SharedDefines.h:436)
	spellAttr0LevelDamageCalculation uint32 = 0x00080000 // SPELL_ATTR0_LEVEL_DAMAGE_CALCULATION (SharedDefines.h:431)
	spellAttr0Negative1              uint32 = 0x04000000 // SPELL_ATTR0_NEGATIVE_1 (SharedDefines.h:438) — forces the spell to be treated as negative
	spellAttr2Unk3                   uint32 = 0x00000008 // SPELL_ATTR2_UNK3 (SharedDefines.h:489) — "Ignore aura scaling"; GetAuraRankForLevel returns the cast rank — ATTR2 is Go's AttributesEx1 (Spell.dbc field 6 = AttributesExB)
	spellAttr3DrainSoul              uint32 = 0x08000000 // SPELL_ATTR3_DRAIN_SOUL (SharedDefines.h:550) — ATTR3 is Go's AttributesEx3 (Spell.dbc field 7 = AttributesExC)

	spellFailedEquippedItemClass         uint8 = 29  // SPELL_FAILED_EQUIPPED_ITEM_CLASS (SharedDefines.h:1011)
	spellFailedEquippedItemClassMainhand uint8 = 30  // SPELL_FAILED_EQUIPPED_ITEM_CLASS_MAINHAND (SharedDefines.h:1012)
	spellFailedEquippedItemClassOffhand  uint8 = 31  // SPELL_FAILED_EQUIPPED_ITEM_CLASS_OFFHAND (SharedDefines.h:1013)
	spellFailedNotInFront                uint8 = 61  // SPELL_FAILED_NOT_INFRONT (SharedDefines.h:1042)
	spellFailedBadTargets                uint8 = 12  // SPELL_FAILED_BAD_TARGETS (SharedDefines.h:992)
	spellFailedBmOrInvisGod              uint8 = 159 // SPELL_FAILED_BM_OR_INVISGOD (SharedDefines.h:1141)
	spellFailedTargetIsPlayer            uint8 = 117 // SPELL_FAILED_TARGET_IS_PLAYER (SharedDefines.h:1099)
	spellFailedAffectingCombat           uint8 = 1
	spellFailedFoodLowLevel              uint8 = 35
	spellFailedNoPet                     uint8 = 84
	spellFailedWrongPetFood              uint8 = 135
	spellFailedNotReady                  uint8 = 67  // SPELL_FAILED_NOT_READY (SharedDefines.h:1049)
	spellFailedDontReport                uint8 = 27  // SPELL_FAILED_DONT_REPORT (SharedDefines.h:1009)
	spellFailedSilenced                  uint8 = 104 // SPELL_FAILED_SILENCED (SharedDefines.h:1086)
	spellFailedCasterDead                uint8 = 23  // SPELL_FAILED_CASTER_DEAD (SharedDefines.h:1003)
	spellFailedNotFishable               uint8 = 58  // SPELL_FAILED_NOT_FISHABLE (SharedDefines.h:1040)
	spellFailedCharmed                   uint8 = 24  // SPELL_FAILED_CHARMED (SharedDefines.h:1006)
	spellFailedConfused                  uint8 = 26  // SPELL_FAILED_CONFUSED (SharedDefines.h:1008)
	spellFailedFleeing                   uint8 = 34  // SPELL_FAILED_FLEEING (SharedDefines.h:1016)
	spellFailedStunned                   uint8 = 108 // SPELL_FAILED_STUNNED (SharedDefines.h:1090)
	spellFailedPacified                  uint8 = 98  // SPELL_FAILED_PACIFIED (SharedDefines.h:1080)
	spellFailedPreventedByMechanic       uint8 = 147 // SPELL_FAILED_PREVENTED_BY_MECHANIC (SharedDefines.h:1129)
	spellFailedCasterAuraState           uint8 = 22  // SPELL_FAILED_CASTER_AURASTATE (SharedDefines.h:1004)
	spellFailedTargetAuraState           uint8 = 111 // SPELL_FAILED_TARGET_AURASTATE (SharedDefines.h:1093)
	spellFailedCantBeDisenchanted        uint8 = 14  // SPELL_FAILED_CANT_BE_DISENCHANTED (SharedDefines.h:996)
	spellFailedLowCastlevel              uint8 = 49  // SPELL_FAILED_LOW_CASTLEVEL (SharedDefines.h:1031)
	spellFailedSummonPending             uint8 = 183 // SPELL_FAILED_SUMMON_PENDING (SharedDefines.h:1165)
	spellFailedTargetNotInInstance       uint8 = 137 // SPELL_FAILED_TARGET_NOT_IN_INSTANCE (SharedDefines.h:1119)
	spellFailedTargetLockedToRaidInst    uint8 = 169 // SPELL_FAILED_TARGET_LOCKED_TO_RAID_INSTANCE (SharedDefines.h:1151)
	spellFailedNotShapeshift             uint8 = 68  // SPELL_FAILED_NOT_SHAPESHIFT (SharedDefines.h:1050)
	spellFailedOnlyShapeshift            uint8 = 94  // SPELL_FAILED_ONLY_SHAPESHIFT (SharedDefines.h:1076)
	spellFailedRequiresSpellFocus        uint8 = 102 // SPELL_FAILED_REQUIRES_SPELL_FOCUS (SharedDefines.h:1084)
	spellFailedTotemCategory             uint8 = 130 // SPELL_FAILED_TOTEM_CATEGORY (SharedDefines.h:1112)
	spellFailedTotems                    uint8 = 131 // SPELL_FAILED_TOTEMS (SharedDefines.h:1113)
	spellFailedNotMounted                uint8 = 64  // SPELL_FAILED_NOT_MOUNTED (SharedDefines.h:1046)
	spellFailedNotOnTaxi                 uint8 = 65  // SPELL_FAILED_NOT_ON_TAXI (SharedDefines.h:1047)
	spellFailedLowLevel                  uint8 = 48  // SPELL_FAILED_LOWLEVEL (SharedDefines.h:1030)
	spellFailedNotKnown                  uint8 = 63  // SPELL_FAILED_NOT_KNOWN (SharedDefines.h:1045)
	spellFailedItemEnchantTradeWindow    uint8 = 182 // SPELL_FAILED_ITEM_ENCHANT_TRADE_WINDOW (SharedDefines.h:1164)
	spellFailedItemGone                  uint8 = 43  // SPELL_FAILED_ITEM_GONE (SharedDefines.h:1025)
	spellFailedNotTrading                uint8 = 71  // SPELL_FAILED_NOT_TRADING (SharedDefines.h:1053)
	spellFailedItemAlreadyEnchanted      uint8 = 42  // SPELL_FAILED_ITEM_ALREADY_ENCHANTED (SharedDefines.h:1024)
	spellFailedItemNotFound              uint8 = 44  // SPELL_FAILED_ITEM_NOT_FOUND (SharedDefines.h:1026)
	spellFailedTooManyOfItem             uint8 = 129 // SPELL_FAILED_TOO_MANY_OF_ITEM (SharedDefines.h:1111)
	spellFailedError                     uint8 = 32  // SPELL_FAILED_ERROR (SharedDefines.h:1014)
	spellFailedNotTradeable              uint8 = 70  // SPELL_FAILED_NOT_TRADEABLE (SharedDefines.h:1052)
	spellFailedOnUseEnchant              uint8 = 170 // SPELL_FAILED_ON_USE_ENCHANT (SharedDefines.h:1152)
	spellFailedMaxSockets                uint8 = 184 // SPELL_FAILED_MAX_SOCKETS (SharedDefines.h:1166)
	spellFailedAuraBounced               uint8 = 9   // SPELL_FAILED_AURA_BOUNCED (SharedDefines.h:991)
	spellFailedNoComboPoints             uint8 = 78  // SPELL_FAILED_NO_COMBO_POINTS (SharedDefines.h:1060)
	spellFailedOnlyBattlegrounds         uint8 = 142 // SPELL_FAILED_ONLY_BATTLEGROUNDS (SharedDefines.h:1124)
	spellFailedNotInArena                uint8 = 151 // SPELL_FAILED_NOT_IN_ARENA (SharedDefines.h:1133)
	spellFailedIncorrectArea             uint8 = 39  // SPELL_FAILED_INCORRECT_AREA (SharedDefines.h:1021)
	spellFailedRequiresArea              uint8 = 101 // SPELL_FAILED_REQUIRES_AREA (SharedDefines.h:1083)
	spellFailedUniqueGlyph               uint8 = 176 // SPELL_FAILED_UNIQUE_GLYPH (SharedDefines.h:1158)
	spellFailedNotInRaidInstance         uint8 = 167 // SPELL_FAILED_NOT_IN_RAID_INSTANCE (SharedDefines.h:1149)
	spellFailedRooted                    uint8 = 103 // SPELL_FAILED_ROOTED (SharedDefines.h:1085)
	spellFailedLowCastLevel              uint8 = 49  // SPELL_FAILED_LOW_CASTLEVEL (SharedDefines.h:1031)
	spellFailedTargetNotLooted           uint8 = 121 // SPELL_FAILED_TARGET_NOT_LOOTED (SharedDefines.h:1103)
	spellFailedTargetUnskinnable         uint8 = 126 // SPELL_FAILED_TARGET_UNSKINNABLE (SharedDefines.h:1108)
	spellFailedTryAgain                  uint8 = 132 // SPELL_FAILED_TRY_AGAIN (SharedDefines.h:1114)
	spellFailedNotInBattleground         uint8 = 166 // SPELL_FAILED_NOT_IN_BATTLEGROUND (SharedDefines.h:1148)
	spellFailedAlreadyHaveSummon         uint8 = 7   // SPELL_FAILED_ALREADY_HAVE_SUMMON (SharedDefines.h:989)
	spellFailedAlreadyHaveCharm          uint8 = 6   // SPELL_FAILED_ALREADY_HAVE_CHARM (SharedDefines.h:988) — caster-side charmed-unit tracking has no Go bridge, named for the charm gate comment
	spellFailedBadImplicitTargets        uint8 = 11  // SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993)
	spellFailedCantBeCharmed             uint8 = 13  // SPELL_FAILED_CANT_BE_CHARMED (SharedDefines.h:995)
	spellFailedHighLevel                 uint8 = 36  // SPELL_FAILED_HIGHLEVEL (SharedDefines.h:1018)
	spellFailedTargetIsPlayerControlled  uint8 = 118 // SPELL_FAILED_TARGET_IS_PLAYER_CONTROLLED (SharedDefines.h:1100)
	spellFailedNoMountsAllowed           uint8 = 83  // SPELL_FAILED_NO_MOUNTS_ALLOWED (SharedDefines.h:1065)
	spellFailedOnlyAboveWater            uint8 = 88  // SPELL_FAILED_ONLY_ABOVEWATER (SharedDefines.h:1070)
	spellFailedTargetFriendly            uint8 = 115 // SPELL_FAILED_TARGET_FRIENDLY (SharedDefines.h:1097)
	spellFailedNotHere                   uint8 = 60  // SPELL_FAILED_NOT_HERE (SharedDefines.h:1042)

	areaFlagNoFlyZone uint32 = 0x20000000 // AREA_FLAG_NO_FLY_ZONE (DBCEnums.h:275) — AreaTableEntry.Flags bit tested by AreaTableEntry::IsFlyable (DBCStructure.h:209)

	spellImplicitTargetUnitPet uint32 = 5 // TARGET_UNIT_PET (SharedDefines.h:1446)

	itemClassWeapon = 2
	itemClassArmor  = 4
	// ITEM_CLASS_TRADE_GOODS (ItemTemplate.h:303): vellum class for the
	// IsFitToSpellRequirements enchant-spell carve-outs (Item.cpp:803-809).
	itemClassTradeGoods = 7

	itemSubclassArmorBuckler = 5
	itemSubclassArmorShield  = 6
	// ItemSubclassTradeGoods vellum subclasses (ItemTemplate.h:444-445).
	itemSubclassArmorEnchantment  = 14
	itemSubclassWeaponEnchantment = 15

	// SpellItemEnchantment.dbc layout (Spell.cpp:6946-6972).
	itemEnchantTypeUseSpell      = 7 // ITEM_ENCHANTMENT_TYPE_USE_SPELL (DBCEnums.h:362)
	itemEnchantTypePrismaticSock = 8 // ITEM_ENCHANTMENT_TYPE_PRISMATIC_SOCKET (DBCEnums.h:363)
	enchantFlagCanSoulbound      = 0x01
	// ENCHANTMENT_CAN_SOULBOUND (DBCEnums.h:394): the EnchantmentSlotMask bit
	// that blocks enchanting a trade-window item.
	maxItemProtoSockets   = 3 // MAX_ITEM_PROTO_SOCKETS (ItemTemplate.h:596)
	maxItemEnchantEffects = 3 // MAX_ITEM_ENCHANTMENT_EFFECTS
	prismaticEnchantIdx   = 18
	// item_instance.enchantments index of the PRISMATIC_ENCHANTMENT_SLOT
	// (slot 6, ItemDefines.h:152) id: 3 ints per slot, slots 0-5 first.
	itemSpellTriggerOnUse        = 0 // ITEM_SPELLTRIGGER_ON_USE (ItemTemplate.h:80)
	itemSpellTriggerOnNoDelayUse = 5 // ITEM_SPELLTRIGGER_ON_NO_DELAY_USE (ItemTemplate.h:90)
	maxItemProtoSpells           = 5 // MAX_ITEM_PROTO_SPELLS

	// InventoryType values (ItemTemplate.h:274, 282-283) for the
	// IsFitToSpellRequirements enchant-spell weapon carve-out (Item.cpp:824).
	invTypeWeapon         = 13
	invTypeWeaponMainhand = 21
	invTypeWeaponOffhand  = 22

	// ITEM_SUBCLASS_MASK_WEAPON_RANGED (ItemTemplate.h:372): bow/gun/
	// crossbow/thrown subclasses; the EquippedItemSubClass DBC field is
	// already a mask (SpellInfo.cpp:843), so this tests the raw field value.
	itemSubclassMaskWeaponRanged uint32 = (1 << 2) | (1 << 3) | (1 << 18) | (1 << 16)

	spellEffectEnergize                = 30
	spellEffectParry                   = 22
	spellEffectPowerBurn               = 62
	spellEffectThreat                  = 63
	spellEffectTriggerSpell            = 64
	spellEffectHealMaxHealth           = 67
	spellEffectCreateItem              = 24
	spellEffectCreateItem2             = 70
	spellEffectLearnSpell              = 36
	spellEffectLearnPetSpell           = 57 // SPELL_EFFECT_LEARN_PET_SPELL (SharedDefines.h:868)
	spellEffectAddExtraAttacks         = 19 // SPELL_EFFECT_ADD_EXTRA_ATTACKS (SharedDefines.h:830)
	spellEffectAddComboPoints          = 80 // SPELL_EFFECT_ADD_COMBO_POINTS (SharedDefines.h:891)
	spellEffectResurrect               = 18
	spellEffectReputation              = 103
	spellEffectQuestComplete           = 16
	spellEffectHealthLeech             = 9
	spellEffectPowerDrain              = 8
	spellEffectCharge                  = 96  // SPELL_EFFECT_CHARGE (SharedDefines.h:907)
	spellEffectSkinning                = 95  // SPELL_EFFECT_SKINNING (SharedDefines.h:906)
	spellEffectOpenLock                = 33  // SPELL_EFFECT_OPEN_LOCK (SharedDefines.h:844)
	spellEffectDispel                  = 38  // SPELL_EFFECT_DISPEL (SharedDefines.h:849)
	spellEffectResurrectPet            = 109 // SPELL_EFFECT_RESURRECT_PET (SharedDefines.h:920)
	spellEffectSummon                  = 28  // SPELL_EFFECT_SUMMON (SharedDefines.h:839)
	spellEffectSummonPet               = 56  // SPELL_EFFECT_SUMMON_PET (SharedDefines.h:867)
	spellEffectCreateTamedPet          = 153 // SPELL_EFFECT_CREATE_TAMED_PET (SharedDefines.h:964)
	spellEffectSummonPlayer            = 85  // SPELL_EFFECT_SUMMON_PLAYER (SharedDefines.h:896)
	spellEffectSummonRafFriend         = 152 // SPELL_EFFECT_SUMMON_RAF_FRIEND (SharedDefines.h:963)
	spellEffectLeap                    = 29  // SPELL_EFFECT_LEAP (SharedDefines.h:840)
	spellEffectTeleportUnitsFaceCaster = 43  // SPELL_EFFECT_TELEPORT_UNITS_FACE_CASTER (SharedDefines.h:854)
	spellEffectJump                    = 41  // SPELL_EFFECT_JUMP (SharedDefines.h:852)
	spellEffectJumpDest                = 42  // SPELL_EFFECT_JUMP_DEST (SharedDefines.h:853)
	spellEffectLeapBack                = 138 // SPELL_EFFECT_LEAP_BACK (SharedDefines.h:949)
	spellEffectTalentSpecSelect        = 162 // SPELL_EFFECT_TALENT_SPEC_SELECT (SharedDefines.h:973)
	// Enchant effects for the IsFitToSpellRequirements isEnchantSpell test
	// (Item.cpp:803: SPELL_EFFECT_ENCHANT_ITEM / _TEMPORARY / _PRISMATIC).
	spellEffectEnchantItem          = 53  // SPELL_EFFECT_ENCHANT_ITEM (SharedDefines.h:864)
	spellEffectEnchantItemTemporary = 54  // SPELL_EFFECT_ENCHANT_ITEM_TEMPORARY (SharedDefines.h:865)
	spellEffectEnchantItemPrismatic = 156 // SPELL_EFFECT_ENCHANT_ITEM_PRISMATIC (SharedDefines.h:967)
	spellEffectDisenchant           = 99  // SPELL_EFFECT_DISENCHANT (SharedDefines.h:910)

	// Summon categories for the generic-summon CheckCast leg
	// (Spell.cpp:5798-5817, SharedDefines.h:3296).
	summonCategoryPet    = 2 // SUMMON_CATEGORY_PET
	summonCategoryPuppet = 3 // SUMMON_CATEGORY_PUPPET — the charm arm has no Go bridge (see checkSummonCast)

	// Implicit targets for the open-lock CheckCast leg (Spell.cpp:5725-5791,
	// SharedDefines.h:1459-1462).
	implicitTargetGameObjectTarget     uint32 = 23 // TARGET_GAMEOBJECT_TARGET
	implicitTargetGameObjectItemTarget uint32 = 26 // TARGET_GAMEOBJECT_ITEM_TARGET

	spellDisarmTrap uint32 = 1842 // Disarm Trap — exempt from the battleground-object gate on traps (Spell.cpp:5754)

	// spellSummonReferAFriend is the refer-a-friend summon spell id carved out
	// of the same-raid gate in the SPELL_EFFECT_SUMMON_PLAYER leg
	// (Spell.cpp:5903, "refer-a-friend spell").
	spellSummonReferAFriend uint32 = 48955

	// Creature template type_flags for the skinning CheckCast leg
	// (Spell.cpp:5707-5724, CreatureData.h:213-222, SharedDefines.h).
	// creatureTypeCritter (CREATURE_TYPE_CRITTER, kill.go:15) already exists:
	// critters skip the looted gate.
	creatureTypeFlagHerbSkinningSkill        uint32 = 0x00000100 // CREATURE_TYPE_FLAG_HERB_SKINNING_SKILL (SharedDefines.h:2737)
	creatureTypeFlagMiningSkinningSkill      uint32 = 0x00000200 // CREATURE_TYPE_FLAG_MINING_SKINNING_SKILL (SharedDefines.h:2738)
	creatureTypeFlagEngineeringSkinningSkill uint32 = 0x00008000 // CREATURE_TYPE_FLAG_ENGINEERING_SKINNING_SKILL (SharedDefines.h:2744)

	// PetTameFailure reasons sent by the summon-pet CheckCast leg's stable
	// block (Spell.cpp:5837-5873, SharedDefines.h:3561-3573).
	petTameNoPetAvailable    uint8 = 7  // PETTAME_NOPETAVAILABLE
	petTameDead              uint8 = 10 // PETTAME_DEAD
	petTameCantControlExotic uint8 = 12 // PETTAME_CANTCONTROLEXOTIC

	// Creature template gates for the summon-pet stable block's IsTameable
	// check (CreatureData.h:230-237, SharedDefines.h:2661/2683/2729/2745).
	creatureTypeBeast           int64 = 1          // CREATURE_TYPE_BEAST
	creatureFamilyNone          int64 = 0          // CREATURE_FAMILY_NONE
	creatureTypeFlagTameablePet int64 = 0x00000001 // CREATURE_TYPE_FLAG_TAMEABLE_PET
	creatureTypeFlagExoticPet   int64 = 0x00010000 // CREATURE_TYPE_FLAG_EXOTIC_PET — IsExotic()

	// Aura type for Player::CanTameExoticPets (Player.h:1826,
	// SpellAuraDefines.h:226).
	spellAuraAllowTamePetType uint32 = 146 // SPELL_AURA_ALLOW_TAME_PET_TYPE

	skillSkinning    uint32 = 393 // SKILL_SKINNING (SharedDefines.h:2985)
	skillHerbalism   uint32 = 182 // SKILL_HERBALISM (SharedDefines.h:2939)
	skillMining      uint32 = 186 // SKILL_MINING (SharedDefines.h:2943)
	skillEngineering uint32 = 202 // SKILL_ENGINEERING (SharedDefines.h:2947)
	skillFishing     uint32 = 356 // SKILL_FISHING (SharedDefines.h:2981)
	skillLockpicking uint32 = 633 // SKILL_LOCKPICKING (SharedDefines.h:2999)
	skillInscription uint32 = 773 // SKILL_INSCRIPTION (SharedDefines.h:3026)
	skillEnchanting  uint32 = 333 // SKILL_ENCHANTING (SharedDefines.h:2978)
	skillNone        uint32 = 0   // SKILL_NONE (SharedDefines.h:2888)

	// Lock.dbc key types (SharedDefines.h:2628-2630) and lock types with a
	// gathering skill (SharedDefines.h:2635-2654) for the CanOpenLock bridge.
	lockKeyItem         uint32 = 1  // LOCK_KEY_ITEM
	lockKeySkill        uint32 = 2  // LOCK_KEY_SKILL
	lockKeySpell        uint32 = 3  // LOCK_KEY_SPELL
	lockTypePicklock    uint32 = 1  // LOCKTYPE_PICKLOCK
	lockTypeHerbalism   uint32 = 2  // LOCKTYPE_HERBALISM
	lockTypeMining      uint32 = 3  // LOCKTYPE_MINING
	lockTypeFishing     uint32 = 19 // LOCKTYPE_FISHING
	lockTypeInscription uint32 = 20 // LOCKTYPE_INSCRIPTION

	// GetConfigMaxSkillValue at the level-80 cap: 300 + (80-60)*75/10
	// (World.h:641) — the orange-lockpick fail-chance ceiling.
	configMaxSkillValue                     int32  = 450
	spellEffectHealMechanical                      = 75  // SPELL_EFFECT_HEAL_MECHANICAL (SharedDefines.h:886)
	spellEffectHealPct                             = 136 // SPELL_EFFECT_HEAL_PCT (SharedDefines.h:947)
	spellEffectEnergizePct                         = 137 // SPELL_EFFECT_ENERGIZE_PCT (SharedDefines.h:948)
	spellAuraMounted                               = 78
	spellAuraModParryPercent                       = 47
	spellAuraModSpellCritChance                    = 57  // SPELL_AURA_MOD_SPELL_CRIT_CHANCE (SpellAuraDefines.h:137)
	spellAuraModSpellCritChanceSchool              = 71  // SPELL_AURA_MOD_SPELL_CRIT_CHANCE_SCHOOL (SpellAuraDefines.h:151)
	spellAuraModCritPct                            = 290 // SPELL_AURA_MOD_CRIT_PCT (SpellAuraDefines.h:370)
	spellAuraRangedAttackPowerAttackerBonus        = 127 // SPELL_AURA_RANGED_ATTACK_POWER_ATTACKER_BONUS (SpellAuraDefines.h:207)
	spellAuraFly                                   = 201 // SPELL_AURA_FLY (SpellAuraDefines.h:281)
	spellAuraModIncreaseMountedFlightSpeed         = 207 // SPELL_AURA_MOD_INCREASE_MOUNTED_FLIGHT_SPEED (SpellAuraDefines.h:287)
	spellAuraConfuse                               = 5
	spellAuraCharm                                 = 6
	spellAuraFear                                  = 7
	spellAuraStun                                  = 12
	spellAuraRoot                                  = 26
	spellAuraStealth                               = 16
	spellAuraInvisibility                          = 18
	spellAuraStealthDetect                         = 17
	spellAuraInvisibilityDetect                    = 19
	spellAuraStealthLevel                          = 154
	spellAuraTrackStealthed                        = 151
	spellAuraConvertRune                           = 249
	spellAuraDamagePercentDone                     = 79
	spellAuraModDamagePercentTaken                 = 87   // SPELL_AURA_MOD_DAMAGE_PERCENT_TAKEN (SpellAuraDefines.h:167)
	spellAuraModMechanicDamageTakenPercent         = 255  // SPELL_AURA_MOD_MECHANIC_DAMAGE_TAKEN_PERCENT (SpellAuraDefines.h:335)
	spellAuraModIgnoreTargetResist                 = 269  // SPELL_AURA_MOD_IGNORE_TARGET_RESIST (SpellAuraDefines.h:349)
	spellAuraModDamageFromCaster                   = 271  // SPELL_AURA_MOD_DAMAGE_FROM_CASTER (SpellAuraDefines.h:351)
	spellAuraDummy                                 = 4    // SPELL_AURA_DUMMY (SpellAuraDefines.h:84)
	spellIconCheatDeath                            = 2109 // Cheat Death dummy aura (Unit.cpp:7078)
	spellSchoolMaskNormal                          = 1    // SPELL_SCHOOL_MASK_NORMAL (SharedDefines.h:324)
	spellAuraAttackPowerPercent                    = 166
	spellAuraRangedAttackPowerPercent              = 167
	spellAuraCastingSpeedNotStack                  = 65
	spellAuraHasteSpells                           = 216
	spellAuraFakeInebriation                       = 304
	spellAuraTransform                             = 56  // SPELL_AURA_TRANSFORM (SpellAuraDefines.h:136)
	spellAuraMechanicImmunity                      = 77  // SPELL_AURA_MECHANIC_IMMUNITY (SpellAuraDefines.h:157)
	spellAuraModMechanicResistance                 = 117 // SPELL_AURA_MOD_MECHANIC_RESISTANCE (SpellAuraDefines.h:197)
	spellAuraModPowerCostSchoolPct                 = 72  // SPELL_AURA_MOD_POWER_COST_SCHOOL_PCT (SpellAuraDefines.h:152)
	spellAuraModPowerCostSchool                    = 73  // SPELL_AURA_MOD_POWER_COST_SCHOOL (SpellAuraDefines.h:153)
	spellAuraModConfuse                            = 5   // SPELL_AURA_MOD_CONFUSE (SpellAuraDefines.h:85)
	spellAuraModFear                               = 7   // SPELL_AURA_MOD_FEAR (SpellAuraDefines.h:87)
	spellAuraModStun                               = 12  // SPELL_AURA_MOD_STUN (SpellAuraDefines.h:92)
	spellAuraStrangulate                           = 298 // SPELL_AURA_STRANGULATE (SpellAuraDefines.h:378)
	spellAuraModSilence                            = 27  // SPELL_AURA_MOD_SILENCE (SpellAuraDefines.h:107)
	spellAuraModPacify                             = 25  // SPELL_AURA_MOD_PACIFY (SpellAuraDefines.h:105)
	spellAuraModPacifySilence                      = 60  // SPELL_AURA_MOD_PACIFY_SILENCE (SpellAuraDefines.h:140)
	spellAuraStateImmunity                         = 38  // SPELL_AURA_STATE_IMMUNITY (SpellAuraDefines.h:118)
	spellAuraDispelImmunity                        = 41  // SPELL_AURA_DISPEL_IMMUNITY (SpellAuraDefines.h:121)
	spellAuraModImmuneAuraApplySchool              = 267 // SPELL_AURA_MOD_IMMUNE_AURA_APPLY_SCHOOL (SpellAuraDefines.h:347)
	spellAuraMechanicImmunityMask                  = 147 // SPELL_AURA_MECHANIC_IMMUNITY_MASK (SpellAuraDefines.h:227)
	spellAuraModRoot                               = 26  // SPELL_AURA_MOD_ROOT (SpellAuraDefines.h:106)
	unitStandFlagCreep                             = 0x02
	playerAuraVisionStealth                        = 0x20
	playerAuraVisionInvis                          = 0x40
	playerFieldByteTrackStealthed           uint32 = 0x00000002
)

// isSelfCastOnly checks if all active spell effects target the caster unit.
func isSelfCastOnly(spell wotlk.Spell) bool {
	hasEffect := false
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		hasEffect = true
		// TrinityCore SpellInfo::IsSelfCast requires TARGET_UNIT_CASTER (1) for every active effect.
		if eff.ImplicitTargetA != 1 {
			return false
		}
	}
	return hasEffect
}

func isAreaEnemySpell(spell wotlk.Spell) bool {
	if !isHarmfulSpell(spell) {
		return false
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.Effect == 129 {
			return true
		}
		if eff.Effect == 27 && eff.ImplicitTargetA == 18 {
			return true
		}
		if isAreaEnemyTargetType(eff.ImplicitTargetA) || isAreaEnemyTargetType(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

func isAreaEnemyTargetType(target uint32) bool {
	switch target {
	case 2, 15, 16, 22, 24, 28, 54, 104:
		return true
	default:
		return false
	}
}

// spellEffectTargetsUnit mirrors SpellEffectInfo::GetUsedTargetObjectType()
// (the static _data table, SpellInfo.cpp:610-614) reporting true when the
// effect's used target object type is TARGET_OBJECT_TYPE_UNIT (SpellInfo.h:103).
// Spell::prepare (Spell.cpp:3178-3188) removes AURA_INTERRUPT_FLAG_SPELL_ATTACK
// auras when the breaking-stealth spell has any unit-typed effect.
func spellEffectTargetsUnit(effect uint32) bool {
	switch effect {
	case 1, 2, 6, 7, 8, 9, 10, 11, 16, 17, 19, 20, 21, 22, 23, 24, 25, 26,
		30, 31, 34, 35, 36, 37, 38, 39, 40, 41, 44, 45, 46, 47, 48, 49,
		51, 52, 55, 57, 58, 59, 60, 62, 63, 65, 66, 67, 68, 70, 71, 73,
		74, 75, 78, 79, 80, 82, 84, 90, 91, 92, 93, 94, 95, 96, 97, 98,
		100, 102, 103, 108, 110, 111, 112, 114, 115, 117, 118, 119, 120,
		121, 123, 124, 125, 126, 128, 129, 130, 131, 132, 133, 134, 136,
		137, 138, 139, 140, 141, 142, 143, 146, 147, 150, 153, 154, 155,
		157, 159, 160, 161, 162, 163, 164:
		return true
	default:
		return false
	}
}

func (s *session) spellAreaEnemyTargets(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) []uint64 {
	if s == nil || s.player == nil || s.server == nil || !isAreaEnemySpell(spell) || s.server.Data == nil {
		return nil
	}
	radius := float32(0)
	cone := false
	destinationCenter := false
	for _, eff := range spell.Effects {
		areaTargetA := isAreaEnemyTargetType(eff.ImplicitTargetA) || (eff.Effect == 27 && eff.ImplicitTargetA == 18)
		areaTargetB := isAreaEnemyTargetType(eff.ImplicitTargetB) || (eff.Effect == 27 && eff.ImplicitTargetB == 18)
		if eff.Effect == 0 || (!areaTargetA && !areaTargetB) {
			continue
		}
		for _, targetType := range []uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			if targetType == 24 || targetType == 54 || targetType == 104 {
				cone = true
			}
			if targetType == 16 || targetType == 18 || targetType == 28 {
				destinationCenter = true
			}
		}
		if value, ok, err := s.server.Data.SpellRadius(eff.RadiusIndex, uint32(s.player.Level)); err == nil && ok && value > radius {
			radius = value
		}
	}
	if radius <= 0 {
		return nil
	}
	centerX, centerY, centerZ := s.player.X, s.player.Y, s.player.Z
	if destinationCenter && target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		centerX, centerY, centerZ = target.Destination.X, target.Destination.Y, target.Destination.Z
	} else if destinationCenter && target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		if destination, ok := s.getCombatTarget(ctx, target.UnitGUID); ok {
			centerX, centerY, centerZ = destination.X, destination.Y, destination.Z
		}
	}
	player := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	targets := make([]uint64, 0)
	seen := make(map[uint64]struct{})
	accept := func(guid uint64, mapID, instanceID uint32, x, y, z float32, faction, unitFlags, flagsExtra, health uint32) {
		if mapID != player.Map || instanceID != player.InstanceID || health == 0 || spellTargetUnitBlocked(spell, unitFlags, flagsExtra, false) || distance3D(x, y, z, centerX, centerY, centerZ) > float64(radius) || !s.server.isAttackableFaction(faction, player) {
			return
		}
		if cone && !hasInArc(s.player.Orientation, s.player.X, s.player.Y, x, y, math.Pi/2) {
			return
		}
		if _, ok := seen[guid]; ok {
			return
		}
		seen[guid] = struct{}{}
		targets = append(targets, guid)
	}
	s.server.motionMu.Lock()
	motionMap := s.server.motionMapLocked(player.Map, player.InstanceID)
	motions := make([]*creatureMotion, 0, len(motionMap))
	for _, motion := range motionMap {
		if motion != nil {
			motions = append(motions, motion)
		}
	}
	s.server.motionMu.Unlock()
	motionGUIDs := make(map[uint64]struct{}, len(motions))
	for _, motion := range motions {
		motionGUIDs[motion.GUID] = struct{}{}
		accept(motion.GUID, motion.Map, motion.InstanceID, motion.X, motion.Y, motion.Z, motion.Faction, motion.UnitFlags, motion.FlagsExtra, motion.Health)
	}
	s.server.sessionsMu.RLock()
	for targetSession := range s.server.sessions {
		if targetSession == s || !targetSession.authed || !targetSession.worldReady.Load() || targetSession.player == nil || targetSession.player.Health == 0 || targetSession.player.Map != player.Map || targetSession.player.InstanceID != player.InstanceID || targetSession.playerAlliance() == s.playerAlliance() {
			continue
		}
		if distance3D(targetSession.player.X, targetSession.player.Y, targetSession.player.Z, centerX, centerY, centerZ) > float64(radius) {
			continue
		}
		if cone && !hasInArc(s.player.Orientation, s.player.X, s.player.Y, targetSession.player.X, targetSession.player.Y, math.Pi/2) {
			continue
		}
		if _, ok := seen[targetSession.playerGUID]; ok {
			continue
		}
		seen[targetSession.playerGUID] = struct{}{}
		targets = append(targets, targetSession.playerGUID)
	}
	s.server.sessionsMu.RUnlock()
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		rows, err := s.server.WorldStore.DB.QueryContext(ctx, `SELECT c.guid, c.id, c.map, c.position_x, c.position_y, c.position_z, COALESCE(t.faction, 0), COALESCE(t.unit_flags, 0), COALESCE(t.flags_extra, 0), c.curhealth FROM creature AS c JOIN creature_template AS t ON t.entry = c.id WHERE c.map = ? AND c.position_x BETWEEN ? AND ? AND c.position_y BETWEEN ? AND ?`, player.Map, float64(centerX-radius), float64(centerX+radius), float64(centerY-radius), float64(centerY+radius))
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var low, entry, mapID, faction, unitFlags, flagsExtra, health int64
				var x, y, z float64
				if rows.Scan(&low, &entry, &mapID, &x, &y, &z, &faction, &unitFlags, &flagsExtra, &health) == nil {
					guid := creatureWorldGUID(uint32(low), uint32(entry))
					if _, hasMotion := motionGUIDs[guid]; !hasMotion {
						accept(guid, uint32(mapID), player.InstanceID, float32(x), float32(y), float32(z), uint32(faction), uint32(unitFlags), uint32(flagsExtra), uint32(health))
					}
				}
			}
		}
	}
	if maxTargets := spell.MaxTargets; maxTargets > 0 {
		// Spell.cpp:1207,1293 — cap the area/cone target list to MaxAffectedTargets
		// plus SPELL_AURA_MOD_MAX_AFFECTED_TARGETS aura modifiers, then
		// Trinity::Containers::RandomResize (Containers.h:77) keeps exactly that
		// many targets chosen uniformly at random; shuffle + truncate draws the
		// same uniform subset.
		maxTargets += uint32(s.totalAuraModifierByAffectMask(spellAuraModMaxAffectedTargets, spell))
		if uint32(len(targets)) > maxTargets {
			rand.Shuffle(len(targets), func(a, b int) { targets[a], targets[b] = targets[b], targets[a] })
			targets = targets[:maxTargets]
		}
	}
	return targets
}

// spellAffectedBySpellFamilyMask mirrors SpellInfo::IsAffected (SpellInfo.cpp:1305)
// as invoked by AuraEffect::IsAffectedOnSpell (SpellAuraEffects.cpp:848): the aura
// spell's SpellFamilyName and the aura effect's SpellClassMask must match the spell.
func spellAffectedBySpellFamilyMask(auraFamilyName uint32, auraFamilyFlags [3]uint32, spell wotlk.Spell) bool {
	if auraFamilyName == 0 {
		return true
	}
	if auraFamilyName != spell.SpellFamilyName {
		return false
	}
	if auraFamilyFlags != [3]uint32{} {
		if auraFamilyFlags[0]&spell.SpellFamilyFlags[0] == 0 && auraFamilyFlags[1]&spell.SpellFamilyFlags[1] == 0 && auraFamilyFlags[2]&spell.SpellFamilyFlags[2] == 0 {
			return false
		}
	}
	return true
}

// totalAuraModifierByAffectMask mirrors Unit::GetTotalAuraModifierByAffectMask
// (Unit.cpp:5017): sums aura amounts of the given type only from aura effects
// whose spell affects the given spell via the spell-family affect mask.
func (s *session) totalAuraModifierByAffectMask(auraType uint32, spell wotlk.Spell) int32 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	var total int32
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped || aura.AuraType != auraType {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != auraType || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if !spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 {
				amount = int32(aura.Amount)
			}
			if amount == 0 {
				amount = effect.BasePoints + 1
			}
			total += amount
		}
	}
	return total
}

func (s *session) calculateSpellPowerCost(spell wotlk.Spell) uint32 {
	cost := spell.ManaCost
	if spell.ManaCostPct > 0 && s.player != nil {
		pType := spell.PowerType
		if pType == 0 { // Mana: calculate percentage from BaseMana per TrinityCore Player::GetCreateMana()
			basePower := s.player.BaseMana
			if basePower == 0 {
				basePower = s.player.MaxPowers[0]
			}
			if basePower == 0 {
				basePower = 100
			}
			cost += (basePower * spell.ManaCostPct) / 100
		} else if pType < 7 {
			basePower := s.player.MaxPowers[pType]
			if basePower == 0 {
				basePower = s.player.Powers[pType]
			}
			if basePower == 0 {
				basePower = 100
			}
			cost += (basePower * spell.ManaCostPct) / 100
		}
	}
	return cost
}

func (s *session) hasSpellReagents(ctx context.Context, spell wotlk.Spell) bool {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return true
	}
	for i := range spell.Reagent {
		if spell.Reagent[i] <= 0 {
			continue
		}
		itemID := spell.Reagent[i]
		need := spell.ReagentCount[i]
		if need == 0 {
			need = 1
		}
		var have int64
		err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count),0) FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ?`, s.playerGUID, int64(itemID)).Scan(&have)
		if err != nil || have < int64(need) {
			return false
		}
	}
	return true
}

func (s *session) takeSpellReagents(ctx context.Context, spell wotlk.Spell) {
	if s == nil || s.player == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	for i := range spell.Reagent {
		if spell.Reagent[i] <= 0 {
			continue
		}
		itemID := spell.Reagent[i]
		need := int64(spell.ReagentCount[i])
		if need == 0 {
			need = 1
		}
		rows, err := s.server.CharactersStore.DB.QueryContext(ctx, `SELECT ci.item, ii.count FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ii.itemEntry = ? ORDER BY ii.count DESC`, s.playerGUID, int64(itemID))
		if err != nil {
			continue
		}
		type stack struct {
			guid  int64
			count int64
		}
		var stacks []stack
		for rows.Next() {
			var st stack
			if rows.Scan(&st.guid, &st.count) == nil {
				stacks = append(stacks, st)
			}
		}
		rows.Close()
		for _, st := range stacks {
			if need <= 0 {
				break
			}
			if st.count <= need {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, `DELETE FROM character_inventory WHERE guid = ? AND item = ?`, s.playerGUID, st.guid)
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, `DELETE FROM item_instance WHERE guid = ?`, st.guid)
				need -= st.count
			} else {
				_, _ = s.server.CharactersStore.DB.ExecContext(ctx, `UPDATE item_instance SET count = count - ? WHERE guid = ?`, need, st.guid)
				need = 0
			}
		}
	}
}

func (s *session) handleCastSpell(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || s.server.Data == nil {
		return true
	}
	reader := protocol.NewReader(payload)
	castID, err := reader.ReadU8()
	if err != nil {
		return false
	}
	spellID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	if s.isDeadOrGhost() {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterDead), true)
		return true
	}
	if s.player.UnitFlags&(unitFlagConfused|unitFlagFleeing) != 0 || s.hasAuraType(spellAuraCharm) {
		failure := spellFailedCharmed
		if s.player.UnitFlags&unitFlagConfused != 0 {
			failure = spellFailedConfused
		} else if s.player.UnitFlags&unitFlagFleeing != 0 {
			failure = spellFailedFleeing
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		return true
	}
	clientCastFlags, err := reader.ReadU8()
	if err != nil {
		return false
	}
	target, err := protocol.ReadSpellTargetData(reader)
	if err != nil {
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "malformed targets", "error", err)
		return false
	}
	if clientCastFlags&0x02 != 0 {
		// SpellCastTargets m_elevation / m_speed (HandleClientCastFlags,
		// SpellHandler.cpp:30): projectile data consumed by
		// SelectImplicitTrajTargets (Spell.cpp:1626). The optional
		// embedded movement block C++ reads next has no Go consumer —
		// movement arrives via the standalone movement opcodes.
		if target.TrajElevation, err = reader.ReadF32(); err != nil {
			return false
		}
		if target.TrajSpeed, err = reader.ReadF32(); err != nil {
			return false
		}
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil {
		s.debug("spell lookup failed", "account", s.accountName, "spell", spellID, "error", err)
		return true
	}
	learned := s.hasActiveSpell(spellID)
	gmMode := s.player.ExtraFlags&playerExtraGMOn != 0 || s.player.PlayerFlags&playerFlagGM != 0
	if !found || spell.Attributes&spellAttributePassive != 0 || !canPlayerCastSpell(learned, gmMode) {
		s.debug("spell cast ignored", "account", s.accountName, "spell", spellID, "reason", spellCastIgnoreReason(spell, found, learned))
		return true
	}
	// Spell::prepare server-side gate (Spell.cpp:3082-3087) via
	// Unit::IsNonMeleeSpellCast(false, true, true, isAutoshoot)
	// (Unit.cpp:3182-3210): only a cast-bar cast blocks a new client-initiated
	// cast; channeled and autorepeat casts never trigger
	// SPELL_FAILED_SPELL_IN_PROGRESS — the new cast breaks them instead
	// (Unit::SetCurrentCastSpell, Unit.cpp:3064).
	if s.genericCastInProgress() && !s.autoShotNonBlockingCast(spellID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 105), true) // SPELL_FAILED_SPELL_IN_PROGRESS = 105
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "another spell cast is in progress")
		return true
	}
	nowUnix := time.Now().Unix()
	if s.isSchoolLocked(spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotReady), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "school lockout active")
		return true
	}
	if s.hasAuraType(18) && (spell.SchoolMask > 1 || spell.SchoolMask == 0) && spell.PreventionType != spellPreventionTypePacify {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedSilenced), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "silenced")
		return true
	}
	// Shapeshift/stance requirements (SpellInfo::CheckShapeshift, SpellInfo.cpp:1455;
	// gated in Spell::CheckCast at Spell.cpp:5247-5275, before the caster-state block):
	// client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	// The GetTalentSpellCost talent-learn exception has no Go equivalent (noted gap).
	if !s.hasIgnoreShapeshiftAura(spell) {
		if shapeResult := s.checkShapeshiftCast(spell); shapeResult != 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, shapeResult), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "shapeshift requirement not met", "result", shapeResult)
			return true
		}
	}
	// Caster aura spell requirements (Spell::CheckCast caster-state block, Spell.cpp:5305-5308):
	// client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if spell.CasterAuraSpell != 0 && !s.hasAura(spell.CasterAuraSpell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "required caster aura missing", "aura", spell.CasterAuraSpell)
		return true
	}
	if spell.ExcludeCasterAuraSpell != 0 && s.hasAura(spell.ExcludeCasterAuraSpell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "excluded caster aura present", "aura", spell.ExcludeCasterAuraSpell)
		return true
	}
	// Caster aura state requirements (Spell::CheckCast caster-state block, Spell.cpp:5298-5304):
	// client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if spell.CasterAuraState != 0 && !s.hasAuraState(spell.CasterAuraState, spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "required caster aura state missing", "state", spell.CasterAuraState)
		return true
	}
	if spell.ExcludeCasterAuraState != 0 && s.hasAuraState(spell.ExcludeCasterAuraState, spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedCasterAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "excluded caster aura state present", "state", spell.ExcludeCasterAuraState)
		return true
	}
	if spell.RequiresSpellFocus != 0 && !s.spellFocusFound(ctx, spell) {
		s.sendCastFailed(ctx, castID, spell, spellFailedRequiresSpellFocus)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "no spell focus object in range", "focus", spell.RequiresSpellFocus)
		return true
	}
	// Caster-aura gate (Spell::CheckCasterAuras, Spell.cpp:6257-6389; called
	// from Spell::CheckCast at Spell.cpp:5509): the caster's UNIT_FIELD_FLAGS
	// CC state blocks the cast unless the spell is immune to caster auras
	// (ATTR6), is usable while in that CC state (ATTR5 mechanic-mask check),
	// or cancels the preventing aura effects. A set mechanic converts the
	// result to SPELL_FAILED_PREVENTED_BY_MECHANIC with the mechanic as the
	// extended packet param (WriteCastResultInfo, Spell.cpp:4102-4107).
	// Client-initiated casts only — triggered casts go through
	// castSpellDirect, not this path. C++ relative order places this right
	// after the spell-focus check (Spell.cpp:5476-5509).
	if result, mechanic := s.checkCasterAuras(spell); result != 0 {
		payload := buildCastFailed(castID, spellID, result)
		if result == spellFailedPreventedByMechanic {
			payload = buildCastFailedParams(castID, spellID, result, mechanic)
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), payload, true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "caster aura state", "failure", result, "mechanic", mechanic)
		return true
	}
	if s.isGCDActive(spell) {
		// Spell::CheckCast (Spell.cpp:5227-5228): DISABLED_WHILE_ACTIVE spells
		// report DONT_REPORT instead of NOT_READY on GCD.
		reason := spellFailedNotReady
		if spell.Attributes&spellAttr0DisabledWhileActive != 0 {
			reason = spellFailedDontReport
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, reason), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "global cooldown active")
		return true
	}
	for _, cd := range s.player.Cooldowns {
		if cd.Spell == spellID && cd.End > nowUnix {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotReady), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "on cooldown")
			return true
		}
	}
	if categoryID := spell.Category; categoryID != 0 {
		for _, cooldown := range s.player.Cooldowns {
			if cooldown.Category == categoryID && cooldown.End > nowUnix && cooldown.CategoryEnd > nowUnix {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotReady), true)
				s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "category cooldown active", "category", categoryID)
				return true
			}
		}
	}
	// CheckCast battleground gate (Spell::CheckCast, Spell.cpp:5433-5437): client-initiated
	// casts only — this path is the client path; the TYPEID_PLAYER arm is vacuous
	// (the session is always a player). Player::InBattleground is
	// m_bgData.bgInstanceID != 0 (Player.h:1906), mirrored by s.bgData.InstanceID
	// (character_battleground_data).
	if spell.AttributesEx3&spellAttr3Battleground != 0 && s.bgData.InstanceID == 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedOnlyBattlegrounds), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "not in battleground")
		return true
	}
	// CheckCast arena gate (Spell::CheckCast, Spell.cpp:5439-5447): spells flagged
	// SPELL_ATTR4_NOT_USABLE_IN_ARENA, or with a recovery time over 10 minutes
	// without SPELL_ATTR4_USABLE_IN_ARENA, cannot be cast in arenas.
	// GetRecoveryTime is max(RecoveryTime, CategoryRecoveryTime)
	// (SpellInfo.cpp:3149-3152); the 10-minute threshold is 10 * MINUTE * IN_MILLISECONDS.
	// The arena test is the caster's Map.dbc entry being a battle arena
	// (MapEntry.IsBattleArena, InstanceType = 4), looked up like sMapStore.
	recoveryTime := spell.RecoveryTime
	if spell.CategoryRecoveryTime > recoveryTime {
		recoveryTime = spell.CategoryRecoveryTime
	}
	if spell.AttributesEx4&spellAttr4NotUsableInArena != 0 ||
		(recoveryTime > 10*60*1000 && spell.AttributesEx4&spellAttr4UsableInArena == 0) {
		if entry, found, err := s.server.Data.Map(s.player.Map); err == nil && found && entry.IsBattleArena() {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotInArena), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "not usable in arena")
			return true
		}
	}
	// CheckCast zone/location leg (Spell::CheckCast, Spell.cpp:5449-5460): the
	// npcbot TYPEID_UNIT arm is vacuous (this is the client-initiated path)
	// and game masters bypass the whole check (Player::IsGameMaster, chat.go
	// isGM pattern). Zone/area are the session's tracked values
	// (s.player.Zone / s.areaID, refreshed on movement and teleports), the Go
	// model of Unit::GetZoneAndAreaId.
	if (s.player.ExtraFlags&playerExtraGMOn) == 0 && (s.player.PlayerFlags&playerFlagGM) == 0 {
		zoneID, areaID := s.player.Zone, s.areaID
		// Area-group leg (SpellInfo::CheckLocation, SpellInfo.cpp:1508-1527):
		// AreaGroupId > 0 requires zone or area membership, else
		// SPELL_FAILED_INCORRECT_AREA. Unknown/missing group data keeps the
		// existing bridge convention (terrain.go) and does not reject.
		if spell.AreaGroupID > 0 {
			if allowed, known, groupErr := s.server.Data.AreaGroupAllows(uint32(spell.AreaGroupID), zoneID, areaID); groupErr == nil && known && !allowed {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedIncorrectArea), true)
				s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "incorrect area group", "zone", zoneID, "area", areaID)
				return true
			}
		}
		// Raid-instance leg (SpellInfo::CheckLocation, SpellInfo.cpp:1552-1556):
		// SPELL_ATTR6_NOT_IN_RAID_INSTANCE fails on raid maps — and when the
		// map entry is missing, matching the C++ !mapEntry arm.
		if spell.AttributesEx6&spellAttr6NotInRaidInstance != 0 {
			if entry, found, err := s.server.Data.Map(s.player.Map); err != nil || !found || entry.IsRaid() {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotInRaidInstance), true)
				s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "not usable in raid instance")
				return true
			}
		}
		// spell_area DB leg (SpellInfo::CheckLocation, SpellInfo.cpp:1558-1567):
		// when the spell has spell_area rows, at least one must fit the
		// player's zone/area (SpellArea::IsFitToRequirements), else
		// SPELL_FAILED_INCORRECT_AREA.
		if rules, hasRules := s.spellAreaRules(ctx, spellID); hasRules && !s.spellAreaRulesFit(ctx, rules, zoneID, areaID) {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedIncorrectArea), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "incorrect area (spell_area)", "zone", zoneID, "area", areaID)
			return true
		}
		// Battleground-spell special cases (SpellInfo::CheckLocation,
		// SpellInfo.cpp:1569-1623). Player::InBattleground is
		// m_bgData.bgInstanceID != 0 (Player.h:1906), mirrored by
		// s.bgData.InstanceID; the player argument is never nil on this path.
		inBattleground := s.bgData.InstanceID != 0
		mapEntry, mapFound, mapErr := s.server.Data.Map(s.player.Map)
		locationOK := true
		switch spellID {
		case 23333, 23335: // Warsong Gulch / Silverwing flag
			locationOK = s.player.Map == 489 && inBattleground
		case 34976: // Netherstorm flag
			locationOK = s.player.Map == 566 && inBattleground
		case 2584, 22011, 22012, 42792, 43681, 44535: // spirit heal / dropped-flag spells
			locationOK = zoneID == WGZoneID || (mapErr == nil && mapFound && mapEntry.IsBattleground() && inBattleground)
		case 44521: // Preparation
			locationOK = mapErr == nil && mapFound && mapEntry.IsBattleground() && inBattleground
			// STATUS_WAIT_JOIN refinement has no bridge: Go battleground
			// queue entries never model WAIT_JOIN (they go 1 -> 3), unlike
			// arena entries whose status syncs to the arena state.
		case 32724, 32725, 35774, 35775: // arena team spells
			locationOK = mapErr == nil && mapFound && mapEntry.IsBattleArena() && inBattleground
		case 32727: // Arena Preparation
			locationOK = false
			if mapErr == nil && mapFound && mapEntry.IsBattleArena() && inBattleground {
				for i := range s.bgQueues {
					if q := &s.bgQueues[i]; q.Active && q.IsArena && q.InstanceID == s.bgData.InstanceID && q.Status == ArenaStatusWaitJoin {
						locationOK = true
						break
					}
				}
			}
		}
		if !locationOK {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedRequiresArea), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "requires area", "zone", zoneID, "area", areaID)
			return true
		}
		// SPELL_ATTR4_CAST_ONLY_IN_OUTLAND (SpellInfo.cpp:1529-1550): no bridge.
		// The strict leg needs AreaTableEntry flyability AND
		// Player::CanFlyInZone (Cold Weather Flying known-spell check via
		// GetVirtualMapForMapAndZone); Go has no WorldMapArea store and no
		// known-spell model for the flight check.
	}
	// CheckCast mounted gate (Spell::CheckCast, Spell.cpp:5477-5488): client-initiated
	// casts only — triggered casts go through castSpellDirect, not this path
	// (TRIGGERED_IGNORE_CASTER_MOUNTED_OR_ON_VEHICLE is never set for client casts).
	// Unit::IsMounted is the UNIT_FLAG_MOUNT unit flag (Unit.h:932), mirrored by
	// unitFlagMount (UNIT_FLAG_MOUNT = 0x08000000, UnitDefines.h:151); the
	// !IsPassive() arm is vacuous here (passive spells return early above) and the
	// TYPEID_PLAYER arm is vacuous (the session is always a player). IsInFlight is
	// the UNIT_STATE_IN_FLIGHT mirror (s.isInFlight, taxi.go).
	if s.player.UnitFlags&unitFlagMount != 0 && spell.Attributes&spellAttr0CastableWhileMounted == 0 {
		reason := spellFailedNotMounted
		if s.isInFlight() {
			reason = spellFailedNotOnTaxi
		}
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, reason), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "mounted cast blocked", "reason_code", reason)
		return true
	}
	// Self-cast only spells (e.g. Demon Skin, Demon Armor, Ice Barrier) must always target the caster
	if isSelfCastOnly(spell) {
		if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 && target.UnitGUID != s.playerGUID {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBadTargets), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "self-cast only spell cannot target other units")
			return true
		}
		target.UnitGUID = s.playerGUID
		target.Flags = protocol.SpellTargetFlagUnit
	}
	if isFishingSpell(spellID) && target.Flags&protocol.SpellTargetFlagDestLocation == 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotFishable), true)
		return true
	}
	for _, effect := range spell.Effects {
		if effect.Effect != 101 {
			continue
		}
		if _, failure := s.checkPetFood(ctx, target.ItemGUID); failure != 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "pet food validation", "failure", failure)
			return true
		}
	}
	// Learn-spell pet gates (Spell::CheckCast per-effect block,
	// Spell.cpp:5570-5618): LEARN_SPELL on TargetA == TARGET_UNIT_PET and
	// LEARN_PET_SPELL require the caster's pet and reject when the learn
	// spell's own SpellLevel exceeds the pet's level.
	if failure := s.checkLearnSpellCast(ctx, spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "learn spell pet validation", "failure", failure)
		return true
	}
	// Apply-glyph duplicate gate (Spell::CheckCast per-effect block,
	// Spell.cpp:5618-5627): socketing a glyph already active in the current
	// talent spec fails with SPELL_FAILED_UNIQUE_GLYPH.
	if failure := s.checkGlyphCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "duplicate glyph", "failure", failure)
		return true
	}
	// Power burn/drain target gate (Spell::CheckCast per-effect block,
	// Spell.cpp:5652-5660): a burn/drain effect fails with
	// SPELL_FAILED_BAD_TARGETS when the unit target (not the caster) uses a
	// power type other than the effect's MiscValue. C++ relative order places
	// this right after the apply-glyph leg; the feed-pet leg runs earlier in
	// Go — order only matters on simultaneous failures, documented.
	if failure := s.checkPowerBurnDrainCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "power burn/drain target power-type mismatch", "failure", failure)
		return true
	}
	// Charge gate (Spell::CheckCast per-effect block, Spell.cpp:5661-5695):
	// a SPELL_EFFECT_CHARGE effect fails with SPELL_FAILED_ROOTED when the
	// caster is rooted, or SPELL_FAILED_DONT_REPORT when the spell needs an
	// explicit unit target but carries none. C++ relative order places this
	// right after the burn/drain leg.
	if failure := s.checkChargeCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "charge validation", "failure", failure)
		return true
	}
	// Skinning gate (Spell::CheckCast per-effect block, Spell.cpp:5707-5724):
	// a SPELL_EFFECT_SKINNING effect fails with SPELL_FAILED_BAD_TARGETS when
	// the target is not a creature, SPELL_FAILED_TARGET_UNSKINNABLE when the
	// corpse lacks the skinnable flag, SPELL_FAILED_TARGET_NOT_LOOTED when a
	// non-critter corpse was not looted, and SPELL_FAILED_LOW_CASTLEVEL when
	// the caster's loot skill is too low for the creature's level. C++
	// relative order places this right after the charge leg.
	if failure := s.checkSkinningCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "skinning validation", "failure", failure)
		return true
	}
	// Open-lock gate (Spell::CheckCast per-effect block, Spell.cpp:5725-5791):
	// a SPELL_EFFECT_OPEN_LOCK effect with a gameobject or gameobject-item
	// implicit target fails with SPELL_FAILED_BAD_TARGETS when the target is
	// missing or not openable, SPELL_FAILED_LOW_CASTLEVEL when the caster's
	// lock skill is too low, and SPELL_FAILED_TRY_AGAIN on the
	// orange-lockpick fail chance. C++ relative order places this right after
	// the skinning leg.
	if failure := s.checkOpenLockCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "open-lock validation", "failure", failure)
		return true
	}
	// Resurrect-pet gate (Spell::CheckCast per-effect block, Spell.cpp:5792-5801):
	// a SPELL_EFFECT_RESURRECT_PET effect fails with
	// SPELL_FAILED_ALREADY_HAVE_SUMMON when the caster's guardian pet is
	// alive. C++ relative order places this right after the open-lock leg.
	if failure := s.checkResurrectPetCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "resurrect-pet validation", "failure", failure)
		return true
	}
	// Generic-summon gate (Spell::CheckCast per-effect block, Spell.cpp:5802-5817):
	// a SPELL_EFFECT_SUMMON effect whose SummonProperties Control is
	// SUMMON_CATEGORY_PET fails with SPELL_FAILED_ALREADY_HAVE_SUMMON when
	// the caster has a pet and the spell lacks SPELL_ATTR1_DISMISS_PET; the
	// charm (puppet) arms have no Go bridge. C++ relative order places this
	// right after the resurrect-pet leg.
	if failure := s.checkSummonCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "summon validation", "failure", failure)
		return true
	}
	// Create-tamed-pet gate (Spell::CheckCast per-effect block,
	// Spell.cpp:5828-5836): a SPELL_EFFECT_CREATE_TAMED_PET effect fails
	// with SPELL_FAILED_BAD_TARGETS when the unit target is not a player,
	// and with SPELL_FAILED_ALREADY_HAVE_SUMMON when the targeted player
	// already has a pet and the spell lacks SPELL_ATTR1_DISMISS_PET.
	// C++ relative order places this right after the generic-summon leg.
	if failure := s.checkCreateTamedPetCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "create-tamed-pet validation", "failure", failure)
		return true
	}
	// Summon-pet gate (Spell::CheckCast per-effect block, Spell.cpp:5837-5873):
	// a SPELL_EFFECT_SUMMON_PET effect fails with SPELL_FAILED_DONT_REPORT
	// when the caster's stable holds a dead or untameable hunter pet for the
	// effect's MiscValue entry, or no pet at all when MiscValue is 0; the
	// pet self-stun (32752) and charm arms have no Go bridge. C++ relative
	// order places this right after the create-tamed-pet leg.
	if failure := s.checkSummonPetCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "summon-pet validation", "failure", failure)
		return true
	}
	// Summon-player gate (Spell::CheckCast per-effect block,
	// Spell.cpp:5892-5928): a SPELL_EFFECT_SUMMON_PLAYER effect (ritual of
	// summoning style) fails with SPELL_FAILED_BAD_TARGETS when the caster's
	// selected target is not another player in the same group (except spell
	// 48955), and the dungeon leg applies when the caster stands in a
	// dungeon. C++ relative order places this right after the summon-pet leg.
	if failure := s.checkSummonPlayerCast(ctx, spell, spellID); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "summon-player validation", "failure", failure)
		return true
	}
	// Recruit-a-friend summon gate (Spell::CheckCast per-effect block,
	// Spell.cpp:5929-5943): a SPELL_EFFECT_SUMMON_RAF_FRIEND effect fails
	// with SPELL_FAILED_BAD_TARGETS unless the selected player is
	// recruiter-linked to the caster in either direction. C++ relative
	// order places this right after the summon-player leg.
	if failure := s.checkSummonRafFriendCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "summon-raf-friend validation", "failure", failure)
		return true
	}
	// Leap / teleport-units-face-caster gate (Spell::CheckCast per-effect
	// block, Spell.cpp:5945-5954): a SPELL_EFFECT_LEAP or
	// SPELL_EFFECT_TELEPORT_UNITS_FACE_CASTER effect fails with
	// SPELL_FAILED_TRY_AGAIN when the caster is in a battleground whose
	// status is not STATUS_IN_PROGRESS ("Do not allow to cast it before
	// BG starts"). C++ relative order places this right after the
	// recruit-a-friend leg.
	if failure := s.checkLeapCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "leap/teleport-before-bg-start validation", "failure", failure)
		return true
	}
	cost := s.calculateSpellPowerCost(spell)
	pType := spell.PowerType
	// Spell::CheckPower (Spell.cpp:6665-6670) checks rune costs when
	// PowerType == POWER_RUNE; RuneCostID (Spell.dbc field 226) was loaded
	// in store.go but never read in world/ — the earlier "wired" claim was
	// an overclaim. Genuine logic now: Spell::CheckRuneCost parity.
	if spell.PowerType == 5 && !s.checkRuneCost(spell, time.Now().UnixMilli()) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 85), true) // SPELL_FAILED_NO_POWER = 85
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "runes on cooldown")
		return true
	}
	if cost > 0 && pType < 7 && s.player.Powers[pType] < cost {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 85), true) // SPELL_FAILED_NO_POWER = 85
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "not enough power", "power", s.player.Powers[pType], "cost", cost)
		return true
	}
	// Target-item arm of Spell::CheckItems (Spell.cpp:6748-6754): runs before
	// the reagent check, matching C++ CheckItems relative order. An item
	// target that no longer resolves fails with SPELL_FAILED_ITEM_GONE;
	// one that does not fit the spell fails with
	// SPELL_FAILED_EQUIPPED_ITEM_CLASS. Client-initiated casts only —
	// triggered casts go through castSpellDirect.
	if failReason := s.checkItemTargetCast(ctx, spell, target); failReason != 0 {
		s.sendCastFailed(ctx, castID, spell, failReason)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "item target requirements not met", "failReason", failReason)
		return true
	}
	// Reagent block of Spell::CheckItems (Spell.cpp:6765-6805): the
	// TRIGGERED_IGNORE_POWER_AND_REAGENT_COST arm is structural (handleCastSpell
	// serves CMSG_CAST_SPELL only; triggered casts go through castSpellDirect,
	// which runs no reagent check), and the outer ITEM_FLAG_NO_REAGENT_COST
	// guard always passes here because m_CastItem is always nil on this path
	// (item casts run through handleUseItem, which runs no CheckCast gates).
	// The remaining skip is Player::CanNoReagentCast — unless the target item
	// is a trade item not owned by the caster, which forces the check anyway.
	// The m_CastItem-is-reagent arms (Spell.cpp:6793-6805) have no bridge for
	// the same reason. Client-initiated casts only.
	checkReagents := !s.canNoReagentCast(spell)
	if !checkReagents && s.tradeItemTargetCast(target) {
		checkReagents = true
	}
	if checkReagents && !s.hasSpellReagents(ctx, spell) {
		s.sendCastFailed(ctx, castID, spell, 100) // SPELL_FAILED_REAGENTS = 100
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "missing reagents")
		return true
	}

	// Totem item/category requirements (Spell::CheckCast, Spell.cpp:6823-6856):
	// run right after the reagent check, matching C++ CheckCast relative order.
	// Client-initiated casts only — triggered casts go through castSpellDirect.
	if failReason := s.checkSpellTotemRequirements(ctx, spell); failReason != 0 {
		s.sendCastFailed(ctx, castID, spell, failReason)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "totem requirements not met", "failReason", failReason)
		return true
	}

	// CREATE_ITEM / CREATE_ITEM_2 arm of the Spell::CheckItems
	// special-effects loop (Spell.cpp:6864-6905): runs right after the totem
	// block, matching C++ CheckItems relative order (totem 6823-6856 →
	// special effects 6858+). Client-initiated casts only — triggered casts
	// go through castSpellDirect.
	if failReason := s.checkSpellCreateItemCast(ctx, spell, target); failReason != 0 {
		s.sendCastFailed(ctx, castID, spell, failReason)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "created-item requirements not met", "failReason", failReason)
		return true
	}

	// ENCHANT_ITEM / ENCHANT_ITEM_PRISMATIC arm of the Spell::CheckItems
	// special-effects loop (Spell.cpp:6917-6994): runs right after the
	// CREATE_ITEM arm, matching C++ CheckItems relative order (CREATE_ITEM
	// 6864 → ENCHANT_ITEM 6917, fallthrough to ENCHANT_ITEM_PRISMATIC 6935).
	// Client-initiated casts only — triggered casts go through castSpellDirect.
	if failReason := s.checkSpellEnchantItemCast(ctx, spell, target); failReason != 0 {
		s.sendCastFailed(ctx, castID, spell, failReason)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "enchant requirements not met", "failReason", failReason)
		return true
	}

	// ENCHANT_ITEM_TEMPORARY arm of the Spell::CheckItems special-effects
	// loop (Spell.cpp:6996-7011): runs right after the ENCHANT_ITEM /
	// ENCHANT_ITEM_PRISMATIC arm, matching C++ CheckItems relative order.
	// Client-initiated casts only — triggered casts go through castSpellDirect.
	if failReason := s.checkSpellEnchantItemTemporaryCast(ctx, spell, target); failReason != 0 {
		s.sendCastFailed(ctx, castID, spell, failReason)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "temporary-enchant requirements not met", "failReason", failReason)
		return true
	}

	// DISENCHANT arm of the Spell::CheckItems special-effects loop
	// (Spell.cpp:7025-7052): runs right after the ENCHANT_ITEM_TEMPORARY
	// arm, matching C++ CheckItems relative order. The ENCHANT_HELD_ITEM
	// arm (Spell.cpp:7022-7024) is a bare break — no CheckItems check — so
	// nothing is bridged for it. Client-initiated casts only — triggered
	// casts go through castSpellDirect.
	if failReason := s.checkSpellDisenchantCast(ctx, spell, target); failReason != 0 {
		s.sendCastFailed(ctx, castID, spell, failReason)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "disenchant requirements not met", "failReason", failReason)
		return true
	}

	if failReason, ok := s.checkSpellEquippedItemRequirements(ctx, spell); !ok {
		s.sendCastFailed(ctx, castID, spell, failReason)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "equipped item requirements not met", "failReason", failReason)
		return true
	}

	isAutoRepeat := (spell.AttributesEx1&0x20 != 0) || spellID == 75 || spellID == 5019
	targetGUID := uint64(0)
	if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		targetGUID = target.UnitGUID
	} else if s.selection != 0 {
		targetGUID = s.selection
	}

	// Target creature-type gate (SpellInfo::CheckTarget, SpellInfo.cpp:1728):
	// sits ahead of the aura-state/aura-spell gates in C++ CheckTarget order.
	// Only checked when a unit target exists, like C++ m_targets.GetUnitTarget().
	// Client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if failReason := s.checkTargetCreatureType(ctx, spell, targetGUID); failReason != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failReason), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "target creature type mismatch", "failReason", failReason)
		return true
	}

	// Target aura spell requirements (SpellInfo::CheckTarget, SpellInfo.cpp:1769-1772):
	// only checked when a unit target exists, like C++ m_targets.GetUnitTarget().
	// Client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if spell.TargetAuraSpell != 0 && targetGUID != 0 && !s.targetHasAura(ctx, targetGUID, spell.TargetAuraSpell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedTargetAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "required target aura missing", "aura", spell.TargetAuraSpell)
		return true
	}
	if spell.ExcludeTargetAuraSpell != 0 && targetGUID != 0 && s.targetHasAura(ctx, targetGUID, spell.ExcludeTargetAuraSpell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedTargetAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "excluded target aura present", "aura", spell.ExcludeTargetAuraSpell)
		return true
	}
	// Target aura state requirements (SpellInfo::CheckTarget, SpellInfo.cpp:1760-1766):
	// only checked when a unit target exists, like C++ m_targets.GetUnitTarget().
	// C++ skips these for vehicle casters and charmer-owned targets; Go has
	// neither concept, so the check always applies here.
	// Client-initiated casts only — triggered casts go through castSpellDirect, not this path.
	if spell.TargetAuraState != 0 && targetGUID != 0 && !s.targetHasAuraState(ctx, targetGUID, spell.TargetAuraState, spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedTargetAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "required target aura state missing", "state", spell.TargetAuraState)
		return true
	}
	if spell.ExcludeTargetAuraState != 0 && targetGUID != 0 && s.targetHasAuraState(ctx, targetGUID, spell.ExcludeTargetAuraState, spell) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedTargetAuraState), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "excluded target aura state present", "state", spell.ExcludeTargetAuraState)
		return true
	}

	// Flying-target gate (SpellInfo::CheckTarget, SpellInfo.cpp:1745-1747):
	// an explicit unit target in flight rejects the cast with
	// SPELL_FAILED_BAD_TARGETS unless the spell carries
	// SPELL_ATTR0_CU_ALLOW_INFLIGHT_TARGET. Only checked when a unit target
	// exists, like the sibling gates above.
	if s.explicitTargetFlyingBlocked(spell, targetGUID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBadTargets), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "target in flight", "target", targetGUID)
		return true
	}

	// Auto-repeat toggle: if already repeating this spell on this target, toggle it off (TC SpellHandler.cpp:420-430)
	if isAutoRepeat && s.autoRepeatSpell == spellID && s.autoRepeatTarget == targetGUID {
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
		buf := protocol.NewBuffer(9)
		buf.WritePackedGUID(s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
		return true
	}

	// Range and Ammo checks for ranged / auto-repeat spells (TC Spell::CheckCast)
	if targetGUID != 0 && targetGUID != s.playerGUID {
		if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
			pReach := float32(1.5)
			if s.player.CombatReach > 0 {
				pReach = s.player.CombatReach
			}
			dist := distance3D(s.player.X, s.player.Y, s.player.Z, tgt.X, tgt.Y, tgt.Z)
			if spellID == 75 { // Auto Shot (TC: Range 114, SPELL_RANGE_RANGED)
				minRange := calcMeleeRange(pReach, tgt.CombatReach)
				if dist < minRange {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 128), true) // SPELL_FAILED_TOO_CLOSE = 128
					return true
				}
				if dist > 35.0 {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 97), true) // SPELL_FAILED_OUT_OF_RANGE = 97
					return true
				}
				if s.player.AmmoID == 0 {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 75), true) // SPELL_FAILED_NO_AMMO = 75
					return true
				}
			} else if spellID == 5019 { // Shoot wand (Range 4)
				if dist > 30.0 {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 97), true) // SPELL_FAILED_OUT_OF_RANGE = 97
					return true
				}
			} else if rangeEntry, ok, _ := s.server.Data.SpellRange(spell.RangeIndex); ok {
				// DBC-driven range check (TC Spell::CheckRange)
				harmful := isHarmfulSpell(spell)
				maxRange := rangeEntry.MaxFriendly
				minRange := rangeEntry.MinFriendly
				if harmful {
					maxRange = rangeEntry.MaxHostile
					minRange = rangeEntry.MinHostile
				}
				if maxRange > 0 && dist > float64(maxRange) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 97), true) // SPELL_FAILED_OUT_OF_RANGE = 97
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "out of range", "dist", dist, "max", maxRange)
					return true
				}
				if minRange > 0 && dist < float64(minRange) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 128), true) // SPELL_FAILED_TOO_CLOSE = 128
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "too close", "dist", dist, "min", minRange)
					return true
				}
			}

			// Positional and facing checks (TrinityCore Spell::CheckCast, Spell.cpp:5200-5300)
			customAttr := s.server.getSpellCustomAttr(spellID)
			// 1. Behind target requirement (SPELL_ATTR0_CU_REQ_CASTER_BEHIND_TARGET = 0x20000):
			// If target has caster in frontal 180° arc, caster is NOT behind target!
			// Returns SPELL_FAILED_NOT_BEHIND = 57 ("You must be behind your target.").
			if customAttr&SpellCustomAttrReqCasterBehindTarget != 0 {
				if hasInArc(tgt.Orientation, tgt.X, tgt.Y, s.player.X, s.player.Y, math.Pi) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 57), true) // SPELL_FAILED_NOT_BEHIND = 57
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "not behind target")
					return true
				}
			}

			// 2. Target facing caster requirement (SPELL_ATTR0_CU_REQ_TARGET_FACING_CASTER = 0x10000):
			// Target must have caster in its frontal 180° arc (e.g. Gouge).
			// Returns SPELL_FAILED_NOT_INFRONT = 61 ("You must be in front of your target.").
			if customAttr&SpellCustomAttrReqTargetFacingCaster != 0 {
				if !hasInArc(tgt.Orientation, tgt.X, tgt.Y, s.player.X, s.player.Y, math.Pi) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedNotInFront), true)
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "target not facing caster")
					return true
				}
			}

			// 3. Caster facing target requirement:
			// If spell has SPELL_FACING_FLAG_INFRONT (0x1) from Spell.dbc field 19 (or Auto Shot / Shoot Wand):
			// Target must be within caster's 120° frontal cone (2*pi/3).
			// Returns SPELL_FAILED_UNIT_NOT_INFRONT = 134 ("Target needs to be in front of you.").
			if (spell.FacingCasterFlags&SpellFacingFlagInfront != 0) || spellID == 75 || spellID == 5019 {
				if !hasInArc(s.player.Orientation, s.player.X, s.player.Y, tgt.X, tgt.Y, 2.0*math.Pi/3.0) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedUnitNotInFront), true)
					s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "target not in front")
					return true
				}
			}
		}
	}

	// Dispel check: if spell has only SPELL_EFFECT_DISPEL effects (and not area-targeting), verify target has dispellable auras
	// Mirrors TrinityCore Spell::CheckCast (Spell.cpp:5520-5565)
	if failReason := s.checkDispelPreCast(spell, targetGUID); failReason != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failReason), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "nothing to dispel")
		return true
	}

	// Spellsteal check: SPELL_EFFECT_STEAL_BENEFICIAL_BUFF (126) fails at cast
	// time when the target carries no stealable aura (Spell::CheckCast,
	// Spell.cpp:5957-5984).
	if failReason := s.checkStealPreCast(spell, targetGUID); failReason != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failReason), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "nothing to steal")
		return true
	}

	// Leap-back / jump gate: SPELL_EFFECT_LEAP_BACK (138),
	// SPELL_EFFECT_JUMP (41) and SPELL_EFFECT_JUMP_DEST (42) fail with
	// SPELL_FAILED_ROOTED when the caster is rooted (Spell::CheckCast,
	// Spell.cpp:5986-6006). C++ relative order places these right after the
	// steal leg.
	if failure := s.checkLeapBackCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "leap-back/jump while rooted", "failure", failure)
		return true
	}

	// Talent spec select gate: SPELL_EFFECT_TALENT_SPEC_SELECT (162) fails
	// with SPELL_FAILED_NOT_IN_BATTLEGROUND when the battleground has
	// already started (Spell::CheckCast, Spell.cpp:6007-6013). C++ relative
	// order places it right after the jump legs.
	if failure := s.checkTalentSpecSelectCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "talent spec change after bg started", "failure", failure)
		return true
	}

	// Possess-pet / charm gates (Spell::CheckCast ApplyAuraName switch,
	// Spell.cpp:6033-6087): SPELL_AURA_MOD_POSSESS_PET fails with
	// SPELL_FAILED_NO_PET when the caster has no pet, and the
	// SPELL_AURA_MOD_POSSESS / SPELL_AURA_MOD_CHARM / SPELL_AURA_AOE_CHARM
	// legs fail with SPELL_FAILED_ALREADY_HAVE_SUMMON when the caster has
	// a pet (non-AoE only) plus the wire unit target gates (vehicle,
	// mounted, charmed, player-controlled, high-level). C++ relative order
	// places the ApplyAuraName switch right after the per-effect switch.
	if failure := s.checkPossessPetCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "possess-pet validation", "failure", failure)
		return true
	}
	if failure := s.checkCharmCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "charm/possess validation", "failure", failure)
		return true
	}

	// Mounted-aura gate (Spell::CheckCast ApplyAuraName switch,
	// Spell.cpp:6088-6114): SPELL_AURA_MOUNTED fails with
	// SPELL_FAILED_ONLY_ABOVEWATER for a flying mount started in water, with
	// SPELL_FAILED_NO_MOUNTS_ALLOWED in a dungeon that disallows mounts, and
	// with SPELL_FAILED_DONT_REPORT while shapeshifted into a
	// disallowed-mount form. C++ relative order places the MOUNTED leg after
	// the charm legs in the ApplyAuraName switch.
	if failure := s.checkMountedCast(ctx, spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "mounted validation", "failure", failure)
		return true
	}

	// Ranged-attack-power-attacker-bonus gate (Spell::CheckCast ApplyAuraName
	// switch, Spell.cpp:6112-6121): an aura-127 effect fails with
	// SPELL_FAILED_BAD_IMPLICIT_TARGETS when the wire target carries no unit
	// GUID, and with SPELL_FAILED_TARGET_FRIENDLY when the unit target is
	// friendly to the caster. C++ relative order places this leg immediately
	// after the SPELL_AURA_MOUNTED leg in the ApplyAuraName switch.
	if failure := s.checkRangedAttackPowerAttackerBonusCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "ranged-attack-power-attacker-bonus validation", "failure", failure)
		return true
	}

	// Flying-mount gate (Spell::CheckCast ApplyAuraName switch,
	// Spell.cpp:6122-6135): an aura-201/aura-207 effect fails with
	// SPELL_FAILED_NOT_HERE in a no-fly zone, or while the zone's
	// battlefield (Wintergrasp) is active. Dead and ghost casters always
	// pass — the C++ comment ("allow always ghost flight spells") is the
	// IsAlive() arm. C++ relative order places this leg immediately after
	// the RANGED_ATTACK_POWER_ATTACKER_BONUS leg in the ApplyAuraName
	// switch.
	if failure := s.checkFlyCast(spell); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "fly validation", "failure", failure)
		return true
	}

	// Periodic mana-leech gate (Spell::CheckCast ApplyAuraName switch,
	// Spell.cpp:6136-6147): an aura-64 (SPELL_AURA_PERIODIC_MANA_LEECH)
	// non-area effect fails with SPELL_FAILED_BAD_IMPLICIT_TARGETS when
	// the wire target carries no unit, and with
	// SPELL_FAILED_BAD_TARGETS when the unit target does not use mana.
	// C++ relative order places this leg immediately after the
	// SPELL_AURA_FLY leg in the ApplyAuraName switch.
	if failure := s.checkPeriodicManaLeechCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "periodic-mana-leech validation", "failure", failure)
		return true
	}

	// Aura-bounced gate (Spell::CheckCast per-effect loop tail,
	// Spell.cpp:6155-6162): a pure-aura non-area spell fails with
	// SPELL_FAILED_AURA_BOUNCED when the unit target already carries a
	// strictly more powerful same-type aura under an EXCLUSIVE_HIGHEST
	// spell group. C++ relative order places this recheck after the
	// ApplyAuraName switch and before the trade-slot block.
	if failure := s.checkAuraBouncedCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "aura-bounced validation", "failure", failure)
		return true
	}

	// Trade-slot gate (Spell::CheckCast, Spell.cpp:6164-6181): the last
	// CheckCast block before the combo-point gate. A cast targeting the
	// trade window fails with SPELL_FAILED_NOT_TRADING when no trade is
	// open, with SPELL_FAILED_BAD_TARGETS when the wire item GUID is not
	// the non-traded slot sentinel, and with
	// SPELL_FAILED_ITEM_ALREADY_ENCHANTED when an enchant is already
	// deferred into the trade. C++ relative order places this block after
	// the per-effect loop's AURA_BOUNCED recheck (Spell.cpp:6155-6162, now
	// bridged by checkAuraBouncedCast), so it wires after the
	// aura-bounced leg.
	if failure := s.checkTradeSlotCast(target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "trade-slot validation", "failure", failure)
		return true
	}

	// Combo-point gate (Spell::CheckCast, Spell.cpp:6186-6209): a spell
	// requiring combo points (REQ_COMBO_POINTS1/2) fails with
	// SPELL_FAILED_NO_COMBO_POINTS when none are banked — against the
	// explicit unit target when the spell needs one, banked points
	// otherwise. C++ relative order places this block after the trade-slot
	// block (Spell.cpp:6164-6181, now bridged by checkTradeSlotCast), so it
	// wires after the trade-slot leg.
	if failure := s.checkComboPointsCast(spell, target); failure != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failure), true)
		s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "combo-point validation", "failure", failure)
		return true
	}

	// Unit::SetCurrentCastSpell (Unit.cpp:3064-3090): registering the new cast
	// breaks the other containers. A generic cast breaks the active channel
	// ("generic spells always break channeled not delayed spells") and any
	// autorepeat that is not Auto Shot — wand Shoot breaks on a new cast
	// while Auto Shot persists through casts, matching in-game behavior. A
	// channeled cast breaks the channel it replaces plus non-Auto-Shot
	// autorepeat; a new wand Shoot (5019) breaks the channel. A new Auto Shot
	// (75) breaks nothing ("only Auto Shoot does not break anything"). Go has
	// no DELAYED channel state, so the C++ withDelayed=false terms are
	// vacuous; C++ sends no packet for these server-side breaks. The gate
	// above already rejected while a cast-bar cast runs, so
	// interruptCurrentCast is the same-container break (SetCurrentCastSpell's
	// InterruptSpell(CSpellType, false)) and a no-op except in the Auto-Shot
	// exception path, which is skipped here.
	if spellID != 75 {
		s.interruptCurrentCast()
		s.interruptCurrentChannel()
	}
	if s.autoRepeatSpell != 0 && s.autoRepeatSpell != 75 {
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
	}
	s.procCastAuras()

	s.lastCastTime = time.Now()
	castTime := s.calculateSpellCastTime(spell)
	// Spell::prepare (Spell.cpp:3139-3149): channeled spells and spells with
	// cast time cannot start while moving, unless the channel allows movement.
	if (isChanneledSpell(spell) || castTime > 0) && s.isMoving && spell.InterruptFlags&spellInterruptFlagMovement != 0 {
		if castTime > 0 || spell.AttributesEx5&spellAttr5CanChannelWhenMoving == 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedMoving), true)
			s.debug("spell cast rejected", "account", s.accountName, "spell", spellID, "reason", "casting while moving")
			return true
		}
	}
	// Spell::prepare (Spell.cpp:3175-3189): stealth breaks at cast start.
	if spell.AttributesEx&spellAttr1NotBreakStealth == 0 {
		s.removeAurasWithInterruptFlags(auraInterruptFlagCast)
		for _, eff := range spell.Effects {
			if spellEffectTargetsUnit(eff.Effect) {
				s.removeAurasWithInterruptFlags(auraInterruptFlagSpellAttack)
				break
			}
		}
	}
	if err := s.write(uint16(protocol.OpcodeSMSG_SPELL_START), protocol.BuildSpellStart(s.playerGUID, s.playerGUID, castID, spellID, spellCastFlagStart, castTime, target), true); err != nil {
		return false
	}

	// Spell::prepare (Spell.cpp:3188-3193) sends SMSG_SPELL_START before
	// TriggerGlobalCooldown.
	s.triggerGlobalCooldown(spell)

	if castTime > 0 {
		// Spell::update (Spell.cpp:3794-3880) drives the PREPARING cast-bar
		// on the server tick (~50ms): each tick revalidates caster/target
		// pointers (UpdatePointers, cancelling when the unit target is
		// gone), checks the movement-interrupt leg, and counts the timer
		// down to cast(!m_casttime). Go has no pointer model and no per-tick
		// update loop, so the cast bar is a single timer and the rechecks
		// move to completion: finishSpellCast revalidates power, runes,
		// range, and line of sight, and movement interrupts land eagerly
		// via interruptSpellsOnMovement (movement.go). A target removed
		// mid-cast fails the range/LoS revalidation at completion rather
		// than cancelling mid-bar.
		s.castMu.Lock()
		castState := &activeCastState{
			CastID:       castID,
			SpellID:      spellID,
			StartAt:      time.Now(),
			CastTimeMs:   castTime,
			InterruptFlg: spell.InterruptFlags,
		}
		castState.Timer = time.AfterFunc(time.Duration(castTime)*time.Millisecond, func() {
			s.castMu.Lock()
			if castState.Cancelled {
				s.castMu.Unlock()
				return
			}
			s.castMu.Unlock()
			s.finishSpellCast(context.Background(), castID, spellID, spell, target, 0, 0)
		})
		s.activeCast = castState
		s.castMu.Unlock()
	} else {
		// Spell::prepare (Spell.cpp:3062-3064) registers the cast on the
		// caster's event queue (_spellEvent fired after CalculateTime(1ms));
		// Go has no event-scheduler model, so instant casts land
		// synchronously here and the cast-bar case is the single AfterFunc
		// timer above. The observable delta is the ~1ms deferred update
		// pass, which no consumer depends on (the commented-out
		// !m_spellInfo->StartRecoveryTime forced-defer leg at
		// Spell.cpp:3189-3193 stayed out of the tree for the same reason).
		s.finishSpellCast(context.Background(), castID, spellID, spell, target, 0, 0)
	}

	s.debug("spell cast accepted", "account", s.accountName, "spell", spellID, "cast_id", castID, "cast_time", castTime, "cost", cost)
	return true
}

// genericCastInProgress mirrors the Spell::prepare server-side gate
// (Spell.cpp:3082-3087) via Unit::IsNonMeleeSpellCast(false, true, true,
// isAutoshoot) (Unit.cpp:3182-3210): only a non-finished cast-bar
// (CURRENT_GENERIC_SPELL) cast blocks a new client-initiated cast.
// Channeled casts (skipChanneled=true) and autorepeat casts
// (skipAutorepeat=true) never trigger SPELL_FAILED_SPELL_IN_PROGRESS — the
// new cast breaks them on registration instead (Unit::SetCurrentCastSpell,
// Unit.cpp:3064). Go arms activeCast only for castTime > 0, so instant casts
// can't be in progress here, matching C++'s skipInstant outcome by
// construction; Go has no DELAYED (missile in flight) state, so that term is
// vacuous. Triggered casts never reach this gate (castSpellDirect), matching
// TRIGGERED_IGNORE_CAST_IN_PROGRESS; C++'s m_cast_count term is exactly the
// client-initiated indicator (SpellHandler.cpp:445).
func (s *session) genericCastInProgress() bool {
	s.castMu.Lock()
	defer s.castMu.Unlock()
	return s.activeCast != nil && !s.activeCast.Cancelled
}

// autoShotNonBlockingCast is the C++ isAutoshoot exception in
// Unit::IsNonMeleeSpellCast (Unit.cpp:3192-3195): a new Auto Shot (75) cast
// is not blocked by an in-progress cast-bar cast carrying
// SPELL_ATTR2_NOT_RESET_AUTO_ACTIONS, and on registration it breaks nothing
// ("only Auto Shoot does not break anything", Unit::SetCurrentCastSpell,
// Unit.cpp:3112-3125).
func (s *session) autoShotNonBlockingCast(spellID uint32) bool {
	if spellID != 75 {
		return false
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if s.activeCast == nil || s.activeCast.Cancelled {
		return false
	}
	cur, found, _ := s.server.Data.Spell(s.activeCast.SpellID)
	return found && cur.AttributesEx1&spellAttr2NotResetAutoActions != 0
}

// validateSpellRange checks DBC range (plus the Auto Shot / Shoot special
// cases) against current caster/target positions. Returns 0 on success or a
// SPELL_FAILED_* code. C++ authority: Spell::CheckRange via Spell::CheckCast.
func (s *session) validateSpellRange(ctx context.Context, spellID uint32, spell wotlk.Spell, targetGUID uint64) uint8 {
	tgt, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok {
		return 0
	}
	pReach := float32(1.5)
	if s.player.CombatReach > 0 {
		pReach = s.player.CombatReach
	}
	dist := distance3D(s.player.X, s.player.Y, s.player.Z, tgt.X, tgt.Y, tgt.Z)
	if spellID == 75 { // Auto Shot
		if dist < calcMeleeRange(pReach, tgt.CombatReach) {
			return 128 // SPELL_FAILED_TOO_CLOSE
		}
		if dist > 35.0 {
			return 97 // SPELL_FAILED_OUT_OF_RANGE
		}
		return 0
	}
	if spellID == 5019 { // Shoot
		if dist > 30.0 {
			return 97 // SPELL_FAILED_OUT_OF_RANGE
		}
		return 0
	}
	rangeEntry, ok, _ := s.server.Data.SpellRange(spell.RangeIndex)
	if !ok {
		return 0
	}
	harmful := isHarmfulSpell(spell)
	maxRange := rangeEntry.MaxFriendly
	minRange := rangeEntry.MinFriendly
	if harmful {
		maxRange = rangeEntry.MaxHostile
		minRange = rangeEntry.MinHostile
	}
	if maxRange > 0 && dist > float64(maxRange) {
		return 97 // SPELL_FAILED_OUT_OF_RANGE
	}
	if minRange > 0 && dist < float64(minRange) {
		return 128 // SPELL_FAILED_TOO_CLOSE
	}
	return 0
}

// checkLearnSpellCast mirrors the per-effect pet gates in Spell::CheckCast
// (Spell.cpp:5570-5618): SPELL_EFFECT_LEARN_SPELL (36) with TargetA ==
// TARGET_UNIT_PET (5) requires the caster's active pet and rejects with
// SPELL_FAILED_LOWLEVEL (48) when the learn spell's own SpellLevel exceeds
// the pet's level; SPELL_EFFECT_LEARN_PET_SPELL (57) requires the unit target
// to be the caster's own pet when a unit target is present. The
// caster-must-be-player terms are vacuous here — client casts always come from
// a player session. Returns the SPELL_FAILED_* result code, 0 on success.
func (s *session) checkLearnSpellCast(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, effect := range spell.Effects {
		switch effect.Effect {
		case spellEffectLearnSpell:
			if effect.ImplicitTargetA != spellImplicitTargetUnitPet {
				continue
			}
		case spellEffectLearnPetSpell:
			if target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0 {
				continue // C++ only gates when a unit target is present (Spell.cpp:5599)
			}
			if s.petNumberForGUID(target.UnitGUID) == 0 {
				return spellFailedBadTargets
			}
		default:
			continue
		}
		petID := s.activePetNumber()
		if petID == 0 {
			return spellFailedNoPet
		}
		if _, found, err := s.server.Data.Spell(effect.TriggerSpell); err != nil || !found {
			return spellFailedNotKnown
		}
		if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
			return 0
		}
		var petLevel int64
		if err := s.server.CharactersStore.DB.QueryRowContext(ctx, "SELECT level FROM character_pet WHERE owner = ? AND id = ?", s.playerGUID, petID).Scan(&petLevel); err != nil {
			return 0 // data anomaly: active pet without a row — no gate instead of a false reject
		}
		if petLevel < 0 {
			petLevel = 0
		}
		if spell.SpellLevel > uint32(petLevel) {
			return spellFailedLowLevel
		}
	}
	return 0
}

// checkGlyphCast mirrors the SPELL_EFFECT_APPLY_GLYPH leg of the CheckCast
// per-effect switch (Spell.cpp:5618-5627): a glyph spell rejects with
// SPELL_FAILED_UNIQUE_GLYPH when the player already has the glyph. C++
// tests HasAura(gp->SpellID); Go sockets glyphs as glyph-property ids in
// player.Glyphs and never applies the aura model, but the glyph spell aura
// is unique per glyph (EffectApplyGlyph casts gp->SpellID on apply,
// SpellEffects.cpp:4075), so a matching active-spec slot is the same
// predicate. The caster-must-be-player term is vacuous — client casts always
// come from a player session. Returns the SPELL_FAILED_* result code, 0 on
// success.
func (s *session) checkGlyphCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil {
		return 0
	}
	spec := s.player.ActiveTalentGroup
	if spec >= 2 {
		spec = 0
	}
	for _, eff := range spell.Effects {
		if eff.Effect != 74 || eff.MiscValue == 0 { // SPELL_EFFECT_APPLY_GLYPH
			continue
		}
		for slot := 0; slot < 6; slot++ {
			if s.player.Glyphs[spec][slot] == uint16(eff.MiscValue) {
				return spellFailedUniqueGlyph
			}
		}
	}
	return 0
}

// checkPowerBurnDrainCast mirrors the SPELL_EFFECT_POWER_BURN /
// SPELL_EFFECT_POWER_DRAIN leg of the CheckCast per-effect switch
// (Spell.cpp:5652-5660): a burn/drain effect rejects with
// SPELL_FAILED_BAD_TARGETS when the caster is a player and the unit target
// (which must differ from the caster) uses a different power type than the
// effect's MiscValue. The caster-must-be-player term is vacuous — client casts
// always come from a player session. Player targets read the shapeshift-aware
// playerPowerType (shapeshift.go); creature targets read the motion PowerType.
// C++ gates only when GetUnitTarget() yields a unit, so unresolvable GUIDs
// (pets have no power-type model) skip the gate. Returns the SPELL_FAILED_*
// result code, 0 on success.
func (s *session) checkPowerBurnDrainCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	if target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0 || target.UnitGUID == s.playerGUID {
		return 0 // no unit target, or the target-is-caster exemption (Spell.cpp:5656)
	}
	targetPower, ok := s.unitTargetPowerType(target.UnitGUID)
	if !ok {
		return 0
	}
	for _, eff := range spell.Effects {
		if eff.Effect != spellEffectPowerBurn && eff.Effect != spellEffectPowerDrain {
			continue
		}
		if targetPower != eff.MiscValue {
			return spellFailedBadTargets
		}
	}
	return 0
}

// unitTargetPowerType resolves the active power type of the unit behind guid:
// playerPowerType for online player targets, the motion PowerType for
// creatures. ok is false when the guid resolves to neither.
func (s *session) unitTargetPowerType(guid uint64) (int32, bool) {
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil {
		return int32(playerPowerType(ts.player)), true
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, guid)
	if motion == nil {
		return 0, false
	}
	return int32(motion.PowerType), true
}

// spellNeedsExplicitUnitTarget mirrors SpellInfo::NeedsExplicitUnitTarget
// (SpellInfo.cpp:1047-1050): (GetExplicitTargetMask() &
// TARGET_FLAG_UNIT_MASK) != 0, with TARGET_FLAG_UNIT_MASK = 0x2 | 0x4 | 0x8
// (SpellInfo.h:70). The PARTY (0x8) and RAID (0x4) bits come from
// spellExplicitUnitTargetMask (targets 35/57, SpellInfo.cpp:226/265); the
// plain-UNIT (0x2) bit comes from TARGET-reference-type entries whose check
// type falls through to TARGET_FLAG_UNIT in
// SpellImplicitTargetInfo::GetExplicitTargetMask (SpellInfo.cpp:134-210):
// TARGET_CHECK_DEFAULT unit entries (25 TARGET_UNIT_TARGET_ANY) and dest
// entries (63-71 TARGET_DEST_TARGET_ANY/front/.../left, 74/75
// TARGET_DEST_TARGET_RANDOM/RADIUS), plus TARGET_CHECK_RAID_CLASS (61
// TARGET_UNIT_TARGET_AREA_RAID_CLASS). All real SPELL_EFFECT_CHARGE spells
// use target 6 (UNIT_ENEMY = 0x80) or 21 (UNIT_ALLY = 0x100), so the mask
// test is vacuous for them — the helper stays for custom-spell fidelity.
func spellNeedsExplicitUnitTarget(spell wotlk.Spell) bool {
	if mask := spellExplicitUnitTargetMask(spell); mask&(targetFlagUnitParty|targetFlagUnitRaid) != 0 {
		return true
	}
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		for _, tgt := range [2]uint32{eff.ImplicitTargetA, eff.ImplicitTargetB} {
			switch tgt {
			case 25, 61, 63, 64, 65, 66, 67, 68, 69, 70, 71, 74, 75:
				return true
			}
		}
	}
	return false
}

// checkChargeCast mirrors the SPELL_EFFECT_CHARGE leg of the CheckCast
// per-effect switch (Spell.cpp:5661-5695). A charge effect rejects with
// SPELL_FAILED_ROOTED when the caster is rooted, and with
// SPELL_FAILED_DONT_REPORT when the spell needs an explicit unit target but
// the cast carries none. Arms: the m_caster->ToUnit() null arm is vacuous
// (client casts always come from a player session); the
// TRIGGERED_IGNORE_CASTER_AURAS arm of the root check is vacuous on this
// path (handleCastSpell serves client-initiated casts only; triggered casts
// go through castSpellDirect). No bridge: the Warbringer script-override arm
// (Spell.cpp:5667-5673 — Unit::IsScriptOverriden reads
// SPELL_AURA_OVERRIDE_CLASS_SCRIPTS (112) aura effects with MiscValue 6953,
// Unit.cpp:4764-4774; Go has no aura-112 model), the LoS arm
// (IsWithinLOSInMap — no LoS/VMap model), and the path/range arm
// (PathGenerator/dtNavMesh are unbuilt, commands_mmaps.go:34; the
// ShortenPathUntilDist back-off has no consumer). Returns the
// SPELL_FAILED_* result code, 0 on success.
func (s *session) checkChargeCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil {
		return 0
	}
	hasCharge := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectCharge {
			hasCharge = true
			break
		}
	}
	if !hasCharge {
		return 0
	}
	if s.rooted { // UNIT_STATE_ROOT (Spell.cpp:5675-5676; s.rooted is the UNIT_STATE_ROOT mirror, conditions.go:584)
		return spellFailedRooted
	}
	if spellNeedsExplicitUnitTarget(spell) && (target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0) {
		return spellFailedDontReport
	}
	return 0
}

// checkSkinningCast mirrors the SPELL_EFFECT_SKINNING leg of the CheckCast
// per-effect block (Spell.cpp:5707-5724): the unit target must be a creature
// (TYPEID_UNIT) carrying UNIT_FLAG_SKINNABLE, the corpse must have been looted
// (Loot::isLooted — the Go Looted marker set when the loot window empties),
// and the caster's profession skill must meet the level-derived requirement.
// The m_caster->GetTypeId() != TYPEID_PLAYER arm is vacuous on the client
// path (handleCastSpell only serves player sessions). GetRequiredLootSkill
// (CreatureData.h:213-222) picks the skill from the creature template's
// type_flags: herb/mining/engineering skinning flags map to their gathering
// skills, otherwise SKILL_SKINNING. Player::GetSkillValue adds the temporary
// bonus to the base value (Player.cpp:6240-6252). When the creature_template
// row is missing the template-dependent arms are skipped, following the
// unknown-data-is-permissive convention (terrain.go); that gap is documented
// in the skill/looted legs below.
func (s *session) checkSkinningCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	hasSkinning := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectSkinning {
			hasSkinning = true
			break
		}
	}
	if !hasSkinning {
		return 0
	}
	if target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0 || uint16(target.UnitGUID>>48) != 0xF130 {
		return spellFailedBadTargets
	}
	motion := s.findCreatureMotion(target.UnitGUID)
	if motion == nil {
		return spellFailedBadTargets
	}
	if motion.UnitFlags&unitFlagSkinnable == 0 { // UNIT_FLAG_SKINNABLE = 0x04000000 (UnitDefines.h:150)
		return spellFailedTargetUnskinnable
	}
	wdb := s.server.WorldStore.DB
	if wdb == nil {
		return 0
	}
	var cType, typeFlags int64
	if err := wdb.QueryRowContext(context.Background(), "SELECT COALESCE(type, 0), COALESCE(type_flags, 0) FROM creature_template WHERE entry = ? LIMIT 1", motion.Entry).Scan(&cType, &typeFlags); err != nil {
		return 0
	}
	if cType != int64(creatureTypeCritter) && !motion.Looted {
		return spellFailedTargetNotLooted
	}
	skillID := skillSkinning
	switch {
	case typeFlags&int64(creatureTypeFlagHerbSkinningSkill) != 0:
		skillID = skillHerbalism
	case typeFlags&int64(creatureTypeFlagMiningSkinningSkill) != 0:
		skillID = skillMining
	case typeFlags&int64(creatureTypeFlagEngineeringSkinningSkill) != 0:
		skillID = skillEngineering
	}
	skillValue := playerSkillTotalValue(s.player, skillID)
	targetLevel := int32(motion.Level)
	var reqValue int32
	if skillValue < 100 {
		reqValue = (targetLevel - 10) * 10
	} else {
		reqValue = targetLevel * 5
	}
	if reqValue > skillValue {
		return spellFailedLowCastLevel
	}
	return 0
}

// skillByLockType mirrors SkillByLockType (SharedDefines.h:3044): lock types
// without a gathering skill (disarm trap, open, treasure, slow open, ...)
// map to SKILL_NONE.
func skillByLockType(lockType uint32) uint32 {
	switch lockType {
	case lockTypePicklock:
		return skillLockpicking
	case lockTypeHerbalism:
		return skillHerbalism
	case lockTypeMining:
		return skillMining
	case lockTypeFishing:
		return skillFishing
	case lockTypeInscription:
		return skillInscription
	default:
		return skillNone
	}
}

// goLockDataIndex mirrors GameObjectTemplate::GetLockId
// (GameObjectData.h:464-484): the lockId lives in data1 for doors and
// buttons, data4 for fishing holes, and data0 for every other lockable type.
func goLockDataIndex(goType uint32) int {
	switch goType {
	case uint32(GameObjectTypeDoor), uint32(GameObjectTypeButton):
		return 1
	case uint32(GameObjectTypeFishingHole):
		return 4
	default:
		return 0
	}
}

// inventoryItemEntryByGUID resolves a wire item GUID (raw or 0x4000-high form)
// to its item_template entry through the owner's character_inventory rows.
func (s *session) inventoryItemEntryByGUID(itemGUID uint64) (uint32, bool) {
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil || itemGUID == 0 {
		return 0, false
	}
	var entry int64
	if err := s.server.CharactersStore.DB.QueryRowContext(context.Background(),
		"SELECT ii.itemEntry FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ci.item = ? LIMIT 1",
		s.playerGUID, int64(itemGUID&0xFFFFFFFFFFFF)).Scan(&entry); err != nil || entry <= 0 {
		return 0, false
	}
	return uint32(entry), true
}

// itemLockIDByEntry reads item_template.lockid for an item entry; unknown
// entries report no lock.
func itemLockIDByEntry(s *session, entry uint32) uint32 {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil || entry == 0 {
		return 0
	}
	var lockID int64
	if err := s.server.WorldStore.DB.QueryRowContext(context.Background(),
		"SELECT COALESCE(lockid, 0) FROM item_template WHERE entry = ? LIMIT 1", entry).Scan(&lockID); err != nil {
		return 0
	}
	return uint32(lockID)
}

// checkOpenLockCast mirrors the SPELL_EFFECT_OPEN_LOCK leg of the CheckCast
// per-effect block (Spell.cpp:5725-5791): an effect-33 cast whose implicit
// target is TARGET_GAMEOBJECT_TARGET (23) or TARGET_GAMEOBJECT_ITEM_TARGET
// (26) needs a GO target (or an openable item target), a resolvable lock,
// and — for skill-keyed locks — the caster's lock skill
// (SkillByLockType, SharedDefines.h:3044) at the lock's required value; the
// orange-lockpick fail chance can still reject with SPELL_FAILED_TRY_AGAIN.
// The m_caster->GetTypeId() != TYPEID_PLAYER arm is vacuous on the client
// path (handleCastSpell only serves player sessions). Item::IsLocked (the
// ITEM_FIELD_FLAG_UNLOCKED check) has no Go model — Go never unlocks items,
// so any item carrying a LockID is treated as locked. The battleground-object
// gate (Spell.cpp:5754-5758, CanUseBattlegroundObject) has no Go bridge and
// is skipped. The m_selfContainer recheck arm of the fail chance is moot —
// Go runs CheckCast once per client cast, which is the initial check C++
// gates the chance on. The LOCK_KEY_ITEM match arm (Spell.cpp:7809) never
// fires here: m_CastItem is nil on the client path (handleCastSpell passes
// no cast item; skeleton-key item casts go through CMSG_USE_ITEM, which
// never runs the CheckCast gates), though the arm still sets reqKey. The
// cast-spell skill bonus (Spell.cpp:7840-7842, skeleton keys) applies to
// item-target spells via the effect's CalcValue.
func (s *session) checkOpenLockCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	var eff *wotlk.SpellEffect
	isItemTarget := false
	for i := range spell.Effects {
		e := &spell.Effects[i]
		if e.Effect != spellEffectOpenLock {
			continue
		}
		switch e.ImplicitTargetA {
		case implicitTargetGameObjectTarget:
		case implicitTargetGameObjectItemTarget:
			isItemTarget = true
		default:
			continue
		}
		eff = e
		break
	}
	if eff == nil {
		return 0
	}
	// GO wire target (Spell.cpp:5733-5735): TARGET_FLAG_GAMEOBJECT flows
	// through UnitGUID in ReadSpellTargetData; the 0xF110 high part marks
	// gameobjects (gameObjectGUID, gameobjects.go:376).
	var goGUID uint64
	if target.Flags&protocol.SpellTargetFlagGameObject != 0 && target.UnitGUID != 0 && uint16(target.UnitGUID>>48) == 0xF110 {
		goGUID = target.UnitGUID
	}
	if !isItemTarget && goGUID == 0 {
		return spellFailedBadTargets
	}
	// pTempItem (Spell.cpp:5737-5744): the trade-window item by slot (the
	// wire "item GUID" on TARGET_FLAG_TRADE_ITEM casts is the trade slot
	// index, tradeSlotCount = 7, trade.go:40) or the caster's inventory item
	// by wire GUID. The trade item only feeds the openable-item gate below;
	// like C++ (m_targets.GetItemTarget resolves inventory items only) the
	// lock-id arm reads inventory items alone.
	var tempItemLockID uint32
	haveTempItem := false
	var invItemLockID uint32
	haveInvItem := false
	if target.Flags&protocol.SpellTargetFlagTradeItem != 0 {
		if slot := uint8(target.ItemGUID); s.trade != nil && s.trade.Partner != nil && s.trade.Partner.trade != nil && slot < tradeSlotCount {
			if it, ok := s.trade.Partner.trade.Items[slot]; ok && it.ItemEntry != 0 {
				tempItemLockID, haveTempItem = itemLockIDByEntry(s, it.ItemEntry), true
			}
		}
	} else if target.Flags&protocol.SpellTargetFlagItem != 0 && target.ItemGUID != 0 {
		if entry, ok := s.inventoryItemEntryByGUID(target.ItemGUID); ok {
			tempItemLockID, haveTempItem = itemLockIDByEntry(s, entry), true
			invItemLockID, haveInvItem = tempItemLockID, true
		}
	}
	// Openable-item gate (Spell.cpp:5749-5752): with TARGET_GAMEOBJECT_ITEM_TARGET
	// and no GO target the item must exist, carry a LockID, and be locked.
	if isItemTarget && goGUID == 0 && (!haveTempItem || tempItemLockID == 0) {
		return spellFailedBadTargets
	}
	// GO template data for the lock-id arm. The wire GO GUID packs the entry
	// above the low guid (gameObjectGUID). A missing template row is
	// permissive (terrain.go convention).
	var goLockID uint32
	goKnown := false
	if goGUID != 0 && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		entry := uint32((goGUID >> 24) & 0xFFFFFF)
		var goType int64
		var data [5]int64
		if err := s.server.WorldStore.DB.QueryRowContext(context.Background(),
			"SELECT type, COALESCE(data0, 0), COALESCE(data1, 0), COALESCE(data2, 0), COALESCE(data3, 0), COALESCE(data4, 0) FROM gameobject_template WHERE entry = ? LIMIT 1",
			entry).Scan(&goType, &data[0], &data[1], &data[2], &data[3], &data[4]); err == nil {
			goKnown = true
			goLockID = uint32(data[goLockDataIndex(uint32(goType))])
		}
	}
	// lockId (Spell.cpp:5760-5771): the GO's lock, else the inventory item's
	// LockID. A GO with no lock is not openable.
	var lockID uint32
	switch {
	case goGUID != 0:
		if !goKnown {
			return 0
		}
		if lockID = goLockID; lockID == 0 {
			return spellFailedBadTargets
		}
	case haveInvItem:
		lockID = invItemLockID
	}
	// CanOpenLock (Spell.cpp:7793-7866).
	if lockID != 0 {
		lock, found, err := s.server.Data.Lock(lockID)
		if err != nil || !found {
			return 0 // unknown Lock.dbc data is permissive
		}
		skillID := skillNone
		var reqSkillValue, skillValue int32
		reqKey := false
		castOK := false
		for j := 0; j < 8 && !castOK; j++ {
			switch lock.Type[j] {
			case lockKeyItem:
				// m_CastItem is nil on the client path, so no entry can
				// match; reqKey still sets (Spell.cpp:7809-7813).
				reqKey = true
			case lockKeySkill:
				reqKey = true
				// wrong locktype, skip (Spell.cpp:7821-7822)
				if uint32(eff.MiscValue) != lock.Index[j] {
					continue
				}
				skillID = skillByLockType(lock.Index[j])
				if skillID != skillNone {
					reqSkillValue = int32(lock.Skill[j])
					// The npcbot arm (Spell.cpp:7836-7839) is vacuous — no
					// creature casters on the client path.
					skillValue = playerSkillTotalValue(s.player, skillID)
					// Skill bonus from the cast spell (Spell.cpp:7840-7842,
					// mostly item spells) on item-target casts.
					if eff.ImplicitTargetA == implicitTargetGameObjectItemTarget || eff.ImplicitTargetB == implicitTargetGameObjectItemTarget {
						skillValue += eff.CalcValue()
					}
					if skillValue < reqSkillValue {
						return spellFailedLowCastLevel
					}
				}
				castOK = true
			case lockKeySpell:
				if spell.ID == lock.Index[j] {
					castOK = true
				}
				reqKey = true
			}
		}
		if !castOK && reqKey {
			return spellFailedBadTargets
		}
		// Fail chance for lockpicking attempts (Spell.cpp:5780-5788):
		// canFailAtMax only for SKILL_LOCKPICKING; irand(skillValue-25,
		// skillValue+37) inclusive both ends is a width-63 roll.
		if skillID != skillNone {
			canFailAtMax := skillID == skillLockpicking
			if canFailAtMax || skillValue < configMaxSkillValue {
				if reqSkillValue > skillValue-25+int32(rand.IntN(63)) {
					return spellFailedTryAgain
				}
			}
		}
	}
	return 0
}

// checkResurrectPetCast mirrors the SPELL_EFFECT_RESURRECT_PET leg of the
// CheckCast per-effect block (Spell.cpp:5792-5801): the cast fails with
// SPELL_FAILED_ALREADY_HAVE_SUMMON when the caster's guardian pet is alive.
// The !unitCaster → SPELL_FAILED_BAD_TARGETS arm is vacuous on the
// client-initiated path (the session is always a player unit), and
// livePetMotion is the Go bridge for GetGuardianPet()+IsAlive — the class pet
// is a Guardian subclass in C++, so the hunter/warlock pet Revive Pet targets
// is exactly this motion; other guardian kinds have no Go model.
func (s *session) checkResurrectPetCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	for _, eff := range spell.Effects {
		if eff.Effect != spellEffectResurrectPet {
			continue
		}
		if s.livePetMotion() != nil {
			return spellFailedAlreadyHaveSummon
		}
	}
	return 0
}

// checkSummonCast mirrors the SPELL_EFFECT_SUMMON (generic summon) leg of the
// CheckCast per-effect block (Spell.cpp:5802-5817): with the effect's
// SummonProperties.dbc Control read via Effects[i].MiscValueB, a
// SUMMON_CATEGORY_PET summon fails with SPELL_FAILED_ALREADY_HAVE_SUMMON
// when the caster already has a pet, unless the spell carries
// SPELL_ATTR1_DISMISS_PET (0x1, SharedDefines.h:449 — the _cast dismissal at
// spells.go:2763 then clears the pet instead). A missing properties entry
// passes, matching C++'s `if (!SummonProperties) break`. The null-caster arm
// is vacuous on the client-initiated path. No bridge: the
// SUMMON_CATEGORY_PUPPET charm arm and the pet leg's fallthrough charm check
// (Unit::GetCharmedGUID → SPELL_FAILED_ALREADY_HAVE_CHARM) — Go has no
// charm/possess model (commands_misc2.go).
func (s *session) checkSummonCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	for _, eff := range spell.Effects {
		if eff.Effect != spellEffectSummon {
			continue
		}
		props, found, err := s.server.Data.SummonProperties(uint32(eff.MiscValueB))
		if err != nil || !found {
			continue // missing DBC row or file: unknown-data-is-permissive (terrain.go convention)
		}
		if props.Control != summonCategoryPet {
			continue // SUMMON_CATEGORY_PUPPET/others: charm arm has no bridge
		}
		if spell.AttributesEx&spellAttr1DismissPet == 0 && s.player.PetGUID != 0 {
			return spellFailedAlreadyHaveSummon
		}
	}
	return 0
}

// checkCreateTamedPetCast mirrors the SPELL_EFFECT_CREATE_TAMED_PET leg of
// the CheckCast per-effect block (Spell.cpp:5828-5836): with a unit target,
// the target must be a player (TYPEID_PLAYER) or the cast fails with
// SPELL_FAILED_BAD_TARGETS; a targeted player that already has a pet fails
// with SPELL_FAILED_ALREADY_HAVE_SUMMON unless the spell carries
// SPELL_ATTR1_DISMISS_PET (0x1, SharedDefines.h:449). Self-targeted casts
// resolve through findSessionByGUID, so the caster's own pet gates the
// replacement the same way C++'s GetUnitTarget() returning the caster does.
// Player targets are matched first (findSessionByGUID); a unit GUID that
// resolves to a creature motion is not TYPEID_PLAYER. An unresolvable GUID
// skips the gate — C++ gates only when GetUnitTarget() yields a unit, and
// pets/guardians have no richer model. Returns the SPELL_FAILED_* result
// code, 0 on success.
func (s *session) checkCreateTamedPetCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	hasTamed := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectCreateTamedPet {
			hasTamed = true
			break
		}
	}
	if !hasTamed {
		return 0
	}
	if target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0 {
		return 0
	}
	if ts := s.server.findSessionByGUID(target.UnitGUID); ts != nil && ts.player != nil {
		if spell.AttributesEx&spellAttr1DismissPet == 0 && ts.player.PetGUID != 0 {
			return spellFailedAlreadyHaveSummon
		}
		return 0
	}
	if s.findCreatureMotion(target.UnitGUID) != nil {
		return spellFailedBadTargets
	}
	return 0
}

// checkSummonPetCast mirrors the SPELL_EFFECT_SUMMON_PET leg of the CheckCast
// per-effect block (Spell.cpp:5837-5873). The m_caster->ToUnit() null arm is
// vacuous on the client-initiated path (the session is always a player
// unit), and the non-player-caster ALREADY_HAVE_SUMMON arm with it —
// creature casters never enter handleCastSpell. No bridge, documented:
// (1) the strict-cast pet self-stun (pet->CastSpell(pet, 32752) so the
// replaced pet does not attack the player) — creature motions carry no
// aura/CC model; (2) the GetCharmedGUID() → SPELL_FAILED_ALREADY_HAVE_CHARM
// arm — Go has no charm/possess model (commands_misc2.go).
//
// The GetPetStable() block bridges through character_pet: C++
// Pet::GetLoadPetInfo(stable, MiscValue, 0, false) resolves the current pet
// (slot 0) or the first unslotted pet (slot 100) whose entry matches
// Effects[i].MiscValue — stabled pets are explicitly excluded ("only from
// current or not stabled pets", Pet.cpp:125) — or, when MiscValue is 0, the
// current pet else the first unslotted pet. A found hunter pet fails with
// SPELL_FAILED_DONT_REPORT plus a SMSG_PET_TAME_FAILURE when it is dead
// (PETTAME_DEAD) or its template is not tameable for this caster
// (PETTAME_CANTCONTROLEXOTIC when tameable-with-exotic, else
// PETTAME_NOPETAVAILABLE); no matching pet with MiscValue == 0 fails with
// PETTAME_NOPETAVAILABLE — a present MiscValue is allowed to create new
// pets. A nil CharactersStore skips the block, matching the
// GetPetStable()-nil pass. Returns the SPELL_FAILED_* result code, 0 on
// success.
func (s *session) checkSummonPetCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	hasSummonPet := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectSummonPet {
			hasSummonPet = true
			break
		}
	}
	if !hasSummonPet {
		return 0
	}
	if s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	cdb := s.server.CharactersStore.DB
	for _, eff := range spell.Effects {
		if eff.Effect != spellEffectSummonPet {
			continue
		}
		misc := uint32(eff.MiscValue)
		var petType, curHealth int64
		var petEntry uint32
		var found bool
		if misc != 0 {
			err := cdb.QueryRowContext(context.Background(),
				"SELECT COALESCE(PetType, 0), COALESCE(curhealth, 0), entry FROM character_pet WHERE owner = ? AND entry = ? AND slot IN (0, 100) ORDER BY slot ASC, id ASC LIMIT 1",
				s.playerGUID, misc).Scan(&petType, &curHealth, &petEntry)
			found = err == nil
		} else {
			err := cdb.QueryRowContext(context.Background(),
				"SELECT COALESCE(PetType, 0), COALESCE(curhealth, 0), entry FROM character_pet WHERE owner = ? AND slot IN (0, 100) ORDER BY slot ASC, id ASC LIMIT 1",
				s.playerGUID).Scan(&petType, &curHealth, &petEntry)
			found = err == nil
		}
		if !found {
			if misc == 0 {
				s.sendTameFailure(petTameNoPetAvailable)
				return spellFailedDontReport
			}
			continue
		}
		if petType != int64(petTypeHunter) {
			continue
		}
		if curHealth == 0 {
			s.sendTameFailure(petTameDead)
			return spellFailedDontReport
		}
		if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			continue // unknown template data is permissive (terrain.go convention)
		}
		var cType, family, typeFlags int64
		err := s.server.WorldStore.DB.QueryRowContext(context.Background(),
			"SELECT COALESCE(type, 0), COALESCE(family, 0), COALESCE(type_flags, 0) FROM creature_template WHERE entry = ? LIMIT 1",
			petEntry).Scan(&cType, &family, &typeFlags)
		if err != nil {
			continue // missing template row: unknown-data-is-permissive
		}
		tameable := creatureTameable(cType, family, typeFlags, s.canTameExoticPets())
		if !tameable {
			if creatureTameable(cType, family, typeFlags, true) {
				s.sendTameFailure(petTameCantControlExotic)
			} else {
				s.sendTameFailure(petTameNoPetAvailable)
			}
			return spellFailedDontReport
		}
	}
	return 0
}

// checkSummonPlayerCast mirrors the SPELL_EFFECT_SUMMON_PLAYER leg of the
// CheckCast per-effect block (Spell.cpp:5892-5928, effect id 85).
func (s *session) checkSummonPlayerCast(ctx context.Context, spell wotlk.Spell, spellID uint32) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	matched := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectSummonPlayer {
			matched = true
			break
		}
	}
	if !matched {
		return 0
	}
	// The caster-TYPEID_PLAYER arm is vacuous on the client-initiated path
	// (the session is always a player).
	//
	// The summon targets the caster's *selected* target, not the wire spell
	// target: Player::GetTarget() is mirrored by the CMSG_SET_SELECTION
	// tracked selection (chat.go).
	if s.selection == 0 {
		return spellFailedBadTargets
	}
	// ObjectAccessor::FindPlayer resolves players only; findSessionByGUID
	// likewise matches player GUIDs alone, so a creature selection (or an
	// offline/unknown GUID) lands here as BAD_TARGETS, and self-selection
	// is rejected like C++.
	target := s.server.findSessionByGUID(s.selection)
	if target == nil || target.player == nil || target == s {
		return spellFailedBadTargets
	}
	// Spell 48955 (refer-a-friend summon) skips the same-group gate;
	// Player::IsInSameRaidWith is "same group" (Player.cpp:2543-2546).
	if spellID != spellSummonReferAFriend && !s.inSameGroupAs(target) {
		return spellFailedBadTargets
	}
	// Player::HasSummonPending has no Go model — no pending-summon state is
	// tracked anywhere — so the SPELL_FAILED_SUMMON_PENDING arm has no
	// bridge (documented; never stubbed).
	//
	// Dungeon leg: the caster's map DBC entry must be a dungeon before the
	// raid-bind / instance-template / access-requirement arms apply. C++
	// dereferences the MapStore entry unconditionally; a missing Go entry is
	// permissive per the unknown-data convention (terrain.go).
	mapEntry, found, err := s.server.Data.Map(s.player.Map)
	if err != nil || !found || !mapEntry.IsDungeon() {
		return 0
	}
	difficulty := s.player.DungeonDifficulty
	if mapEntry.IsRaid() {
		difficulty = s.player.RaidDifficulty
		// Raid-lock arm: both sides bound to this map+difficulty, the
		// target's bind permanent, and different instance ids. The binds
		// come from character_instance/instance (instanceBindsForCharacter),
		// the Go equivalent of the BoundInstancesMap arms.
		if targetBind := findInstanceBind(s.instanceBindsForCharacter(ctx, target.playerGUID), mapEntry.ID, difficulty); targetBind != nil {
			if casterBind := findInstanceBind(s.instanceBindsForCharacter(ctx, s.playerGUID), mapEntry.ID, difficulty); casterBind != nil {
				if targetBind.permanent && targetBind.instanceID != casterBind.instanceID {
					return spellFailedTargetLockedToRaidInst
				}
			}
		}
	}
	// sObjectMgr::GetInstanceTemplate(mapId) — the instance_template world
	// row must exist for the dungeon.
	if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return 0
	}
	var tmplMap int64
	if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT map FROM instance_template WHERE map = ?", mapEntry.ID).Scan(&tmplMap); err != nil {
		return spellFailedTargetNotInInstance
	}
	// Player::Satisfy(GetAccessRequirement(mapId, difficulty)) — the
	// level/item/quest access-requirement model has no Go bridge (documented;
	// never stubbed).
	return 0
}

// checkSummonRafFriendCast mirrors the SPELL_EFFECT_SUMMON_RAF_FRIEND leg of
// the Spell::CheckCast per-effect switch (Spell.cpp:5929-5943).
func (s *session) checkSummonRafFriendCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	matched := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectSummonRafFriend {
			matched = true
			break
		}
	}
	if !matched {
		return 0
	}
	// The caster-TYPEID_PLAYER arm is vacuous on the client-initiated path
	// (the session is always a player).
	//
	// The summon targets the caster's selected player: Player::GetTarget()
	// is the CMSG_SET_SELECTION tracked selection (chat.go), and
	// Player::GetSelectedPlayer is ObjectAccessor::FindConnectedPlayer
	// (Player.cpp:22866-22871) — the Go findSessionByGUID bridge likewise
	// resolves online player sessions only, so a zero, creature, offline,
	// or unknown selection lands here as BAD_TARGETS.
	if s.selection == 0 {
		return spellFailedBadTargets
	}
	target := s.server.findSessionByGUID(s.selection)
	if target == nil || target.player == nil {
		return spellFailedBadTargets
	}
	// The recruiter link holds in either direction:
	// target->GetSession()->GetRecruiterId() == caster's account id, or
	// target's account id == caster's recruiter id (WorldSession.h:580).
	// recruiterID is populated from account.recruiter at auth
	// (WorldSocket.cpp:267 bridge); a 0 recruiter id can never match a
	// real (non-zero) account id, so an unlinked pair fails here.
	if target.recruiterID != s.accountID && target.accountID != s.recruiterID {
		return spellFailedBadTargets
	}
	return 0
}

// checkLeapCast mirrors the SPELL_EFFECT_LEAP /
// SPELL_EFFECT_TELEPORT_UNITS_FACE_CASTER leg of the CheckCast
// per-effect switch (Spell.cpp:5945-5954): "Do not allow to cast it
// before BG starts." A caster in a battleground whose status is not
// STATUS_IN_PROGRESS fails with SPELL_FAILED_TRY_AGAIN.
//
// The caster-TYPEID_PLAYER arm is vacuous on the client-initiated path
// (the session is always a player). Player::GetBattleground bridges as
// s.bgData.InstanceID != 0 (the Player.h:1906 InBattleground pattern);
// bg->GetStatus() bridges as the matching s.bgQueues entry's Status.
// Arena queue entries use the arena status scale
// (ArenaStatusInProgress = battle active, battleground_arena.go), and
// battleground entries the BG scale (STATUS_IN_PROGRESS = 3,
// Battleground.h:181). A missing queue entry is permissive
// (unknown-data-is-permissive, terrain.go convention) — C++ always has
// a status on a live battleground.
func (s *session) checkLeapCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	matched := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectLeap || eff.Effect == spellEffectTeleportUnitsFaceCaster {
			matched = true
			break
		}
	}
	if !matched {
		return 0
	}
	if s.bgData.InstanceID == 0 {
		return 0
	}
	for i := range s.bgQueues {
		q := &s.bgQueues[i]
		if !q.Active || q.InstanceID != s.bgData.InstanceID {
			continue
		}
		inProgress := q.Status == 3 // STATUS_IN_PROGRESS (Battleground.h:181)
		if q.IsArena {
			inProgress = q.Status == ArenaStatusInProgress
		}
		if !inProgress {
			return spellFailedTryAgain
		}
		return 0
	}
	return 0
}

// checkLeapBackCast mirrors the SPELL_EFFECT_LEAP_BACK / SPELL_EFFECT_JUMP /
// SPELL_EFFECT_JUMP_DEST legs of the CheckCast per-effect switch
// (Spell.cpp:5986-6006): a rooted caster fails with SPELL_FAILED_ROOTED.
//
// The m_caster->ToUnit() null arm is vacuous on the client-initiated path
// (the session is always a player unit), and the LEAP_BACK
// SPELL_FAILED_DONT_REPORT arm for non-player casters is vacuous for the
// same reason — the caster is always a player here, so the JUMP/JUMP_DEST
// and LEAP_BACK root gates are identical in Go.
func (s *session) checkLeapBackCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil {
		return 0
	}
	matched := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectLeapBack || eff.Effect == spellEffectJump || eff.Effect == spellEffectJumpDest {
			matched = true
			break
		}
	}
	if !matched {
		return 0
	}
	if s.rooted { // UNIT_STATE_ROOT (conditions.go:584, same mirror as checkChargeCast)
		return spellFailedRooted
	}
	return 0
}

// checkTalentSpecSelectCast mirrors the SPELL_EFFECT_TALENT_SPEC_SELECT leg
// of the CheckCast per-effect switch (Spell.cpp:6007-6013): the spec cannot
// be changed once the arena/battleground has started. A caster in a
// battleground whose status is STATUS_IN_PROGRESS fails with
// SPELL_FAILED_NOT_IN_BATTLEGROUND.
//
// The caster-TYPEID_PLAYER arm is vacuous on the client-initiated path
// (the session is always a player). Player::GetBattleground bridges as
// s.bgData.InstanceID != 0 and bg->GetStatus() as the matching s.bgQueues
// entry's Status — the same bridge as checkLeapCast (arena entries use
// ArenaStatusInProgress, battleground entries the BG scale). A missing
// queue entry is permissive (unknown-data-is-permissive, terrain.go
// convention) — C++ always has a status on a live battleground.
func (s *session) checkTalentSpecSelectCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	matched := false
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectTalentSpecSelect {
			matched = true
			break
		}
	}
	if !matched {
		return 0
	}
	if s.bgData.InstanceID == 0 {
		return 0
	}
	for i := range s.bgQueues {
		q := &s.bgQueues[i]
		if !q.Active || q.InstanceID != s.bgData.InstanceID {
			continue
		}
		inProgress := q.Status == 3 // STATUS_IN_PROGRESS (Battleground.h:181)
		if q.IsArena {
			inProgress = q.Status == ArenaStatusInProgress
		}
		if inProgress {
			return spellFailedNotInBattleground
		}
		return 0
	}
	return 0
}

// checkPossessPetCast mirrors the SPELL_AURA_MOD_POSSESS_PET leg of the
// CheckCast ApplyAuraName switch (Spell.cpp:6033-6043): a possess-pet spell
// (Eyes of the Beast) fails with SPELL_FAILED_NO_PET when the caster has no
// pet, and with SPELL_FAILED_CHARMED when the pet is itself charmed.
//
// The caster-TYPEID_PLAYER arm is vacuous on the client-initiated path (the
// session is always a player). Player::GetPet bridges as livePetMotion()
// (the GetGuardianPet()+IsAlive bridge from the resurrect-pet leg), and
// pet->GetCharmerGUID() as the motion charm state (charmCreature,
// creaturemotion.go) — Eyes of the Beast on a GM-charmed pet fails CHARMED,
// matching C++. Returns the SPELL_FAILED_* result code, 0 on success.
func (s *session) checkPossessPetCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil {
		return 0
	}
	matched := false
	for _, eff := range spell.Effects {
		if eff.Aura == spellAuraModPossessPet {
			matched = true
			break
		}
	}
	if !matched {
		return 0
	}
	pet := s.livePetMotion()
	if pet == nil {
		return spellFailedNoPet
	}
	if pet.Charmed || pet.CharmerGUID != 0 {
		return spellFailedCharmed
	}
	return 0
}

// checkCharmCast mirrors the SPELL_AURA_MOD_POSSESS / SPELL_AURA_MOD_CHARM /
// SPELL_AURA_AOE_CHARM leg of the CheckCast ApplyAuraName switch
// (Spell.cpp:6044-6087): caster-side charm-state gates plus the wire unit
// target gates (vehicle, mounted, charmed, player-controlled, level).
//
// Caster side (session is always a player unit, so the unitCaster-null arm
// and the m_originalCaster redirect are vacuous): unitCaster->GetCharmerGUID
// has no bridge — Go players carry no charm state — and
// unitCaster->GetCharmedGUID() has no bridge either (caster-side charmed-unit
// tracking is absent; motion.CharmerGUID only marks the creature side), both
// documented as no-bridge. unitCaster->GetPetGUID() bridges as
// s.player.PetGUID for the MOD_CHARM/MOD_POSSESS (non-AoE)
// SPELL_FAILED_ALREADY_HAVE_SUMMON arm, gated on
// !SPELL_ATTR1_DISMISS_PET (spellAttr1DismissPet).
//
// Target side (GetUnitTarget() != null mirrors as a wire unit GUID):
//   - TYPEID_UNIT && IsVehicle → SPELL_FAILED_BAD_IMPLICIT_TARGETS: Go
//     motions have no vehicle-kit model — documented no-bridge.
//   - IsMounted → SPELL_FAILED_CANT_BE_CHARMED: player targets via
//     isPlayerMounted() (commands_misc.go), creature targets via
//     UNIT_FLAG_MOUNT on the motion UnitFlags (Unit::IsMounted,
//     Unit.h:932).
//   - GetCharmerGUID → SPELL_FAILED_CHARMED: motion charm state
//     (creaturemotion.go); player targets carry no charm model.
//   - GetOwner() && owner TYPEID_PLAYER →
//     SPELL_FAILED_TARGET_IS_PLAYER_CONTROLLED: motion.OwnerGUID resolving
//     to an online player session (findSessionByGUID); player targets have
//     no owner in Go, matching the C++ GetOwner()-null pass.
//   - CalculateDamage(i) → SPELL_FAILED_HIGHLEVEL when value != 0 and the
//     target level exceeds it: eff.CalcValueForLevel(spell, casterLevel)
//     is the Go CalculateDamage bridge, player levels from
//     ts.player.Level, creature levels from motion.Level. Unresolvable
//     GUIDs skip the target gates, mirroring C++ gating only when
//     GetUnitTarget() yields a unit.
//
// Returns the SPELL_FAILED_* result code, 0 on success. None of these
// results carry extra WriteCastResultInfo params, so castFailedExtParams
// needs no case (verified Spell.cpp:3974-4160).
func (s *session) checkCharmCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	matched, nonAoe := false, false
	for _, eff := range spell.Effects {
		switch eff.Aura {
		case spellAuraModPossess:
			matched, nonAoe = true, true
		case spellAuraCharm: // SPELL_AURA_MOD_CHARM (SpellAuraDefines.h:86)
			matched, nonAoe = true, true
		case spellAuraAoeCharm:
			matched = true
		}
	}
	if !matched {
		return 0
	}
	if nonAoe && spell.AttributesEx&spellAttr1DismissPet == 0 && s.player.PetGUID != 0 {
		return spellFailedAlreadyHaveSummon
	}
	if target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0 {
		return 0
	}
	guid := target.UnitGUID
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil {
		// Player target: mounted and level gates bridge; charm and
		// player-controlled arms are vacuous (no player charm or owner model).
		if ts.isPlayerMounted() {
			return spellFailedCantBeCharmed
		}
		for _, eff := range spell.Effects {
			if value := eff.CalcValueForLevel(spell, uint32(s.player.Level)); value != 0 && int32(ts.player.Level) > value {
				return spellFailedHighLevel
			}
		}
		return 0
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, guid)
	if motion == nil {
		return 0
	}
	// TYPEID_UNIT && IsVehicle arm: no vehicle-kit model on motions — no bridge.
	if motion.UnitFlags&unitFlagMount != 0 { // UNIT_FLAG_MOUNT (UnitDefines.h:151)
		return spellFailedCantBeCharmed
	}
	if motion.Charmed || motion.CharmerGUID != 0 {
		return spellFailedCharmed
	}
	if motion.OwnerGUID != 0 && s.server.findSessionByGUID(motion.OwnerGUID) != nil {
		return spellFailedTargetIsPlayerControlled
	}
	for _, eff := range spell.Effects {
		if value := eff.CalcValueForLevel(spell, uint32(s.player.Level)); value != 0 && int32(motion.Level) > value {
			return spellFailedHighLevel
		}
	}
	return 0
}

// checkMountedCast mirrors the SPELL_AURA_MOUNTED leg of the CheckCast
// ApplyAuraName switch (Spell.cpp:6088-6114).
//
//   - unitCaster null → SPELL_FAILED_BAD_TARGETS: vacuous (the session is
//     always a player unit).
//   - in-water flying mount → SPELL_FAILED_ONLY_ABOVEWATER: s.isSwimming is
//     the IsInWater bridge (movement.go) and spellHasAura(spell,
//     spellAuraMountedFlightSpeed) is SpellInfo::HasAura(207, spells.go:7415).
//   - dungeon mountability: allowMount = !IsDungeon() || IsBattlegroundOrArena()
//     with the instance_template.allowMount row overriding
//     (sObjectMgr::GetInstanceTemplate, ObjectMgr.cpp); the session is always
//     a player, so !allowMount && spell.AreaGroupID == 0 →
//     SPELL_FAILED_NO_MOUNTS_ALLOWED. Missing map entry, missing row, or a
//     query error is permissive (terrain.go convention), matching the C++
//     null-template pass that leaves the computed value standing.
//   - IsInDisallowedMountForm → SPELL_FAILED_DONT_REPORT: a live shapeshift
//     form whose SpellShapeshiftForm.dbc flags lack 0x1 rejects
//     (Unit.cpp:9170-9185); a missing form row rejects too. The
//     transform-spell carve-out has no bridge (Go tracks no transform-spell
//     state), and the native/display-ID arms are vacuous on the client path
//     (Go never changes the player display ID for an aura). C++ also sends
//     MountResult::Shapeshifted before the cast result; the Go protocol has
//     no SMSG_MOUNT_RESULT opcode, so the DONT_REPORT cast result is the
//     only feedback — documented, not stubbed.
//
// Returns the SPELL_FAILED_* result code, 0 on success. Neither failure
// code carries extra WriteCastResultInfo params, so castFailedExtParams
// needs no case (verified Spell.cpp:3974-4160).
func (s *session) checkMountedCast(ctx context.Context, spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	matched := false
	for _, eff := range spell.Effects {
		if eff.Aura == spellAuraMounted {
			matched = true
			break
		}
	}
	if !matched {
		return 0
	}
	if s.isSwimming && spellHasAura(spell, spellAuraMountedFlightSpeed) {
		return spellFailedOnlyAboveWater
	}
	allowMount := true
	if entry, found, err := s.server.Data.Map(s.player.Map); err == nil && found {
		allowMount = !entry.IsDungeon() || teleIsBattlegroundOrArena(entry)
	}
	if s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		var dbAllow int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT allowMount FROM instance_template WHERE map = ?", s.player.Map).Scan(&dbAllow); err == nil {
			allowMount = dbAllow != 0
		}
	}
	if !allowMount && spell.AreaGroupID == 0 {
		return spellFailedNoMountsAllowed
	}
	if form := s.player.ShapeshiftForm; form != 0 {
		shape, found, err := s.server.Data.ShapeshiftForm(uint32(form))
		if err != nil || !found || shape.Flags&0x1 == 0 {
			return spellFailedDontReport
		}
	}
	return 0
}

// checkRangedAttackPowerAttackerBonusCast mirrors the
// SPELL_AURA_RANGED_ATTACK_POWER_ATTACKER_BONUS leg of the CheckCast
// ApplyAuraName switch (Spell.cpp:6112-6121): no unit target →
// SPELL_FAILED_BAD_IMPLICIT_TARGETS, a unit target friendly to the caster →
// SPELL_FAILED_TARGET_FRIENDLY. The C++ comment ("can be cast at
// non-friendly unit or own pet/charm") is narrower than the code —
// IsFriendlyTo covers pet/charm too, so any friendly target is rejected.
//
//   - player targets: the session-level friendliness model
//     (isFriendlyToTarget, dispel.go — team + duel-hostility + own-pet) is
//     the Unit::IsFriendlyTo bridge; a self-target is always friendly.
//   - creature targets: the faction-template friendliness model
//     (explicit_target_faction.go:88) is the bridge.
//   - unresolvable GUIDs skip the friendliness gate: C++ gates only when
//     GetUnitTarget() yields a unit (pets have no faction model).
//
// Returns the SPELL_FAILED_* result code, 0 on success. TARGET_FRIENDLY
// carries no extra WriteCastResultInfo params, so castFailedExtParams needs
// no case (verified Spell.cpp:3974-4160).
func (s *session) checkRangedAttackPowerAttackerBonusCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	matched := false
	for _, eff := range spell.Effects {
		if eff.Aura == spellAuraRangedAttackPowerAttackerBonus {
			matched = true
			break
		}
	}
	if !matched {
		return 0
	}
	if target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0 {
		return spellFailedBadImplicitTargets
	}
	guid := target.UnitGUID
	if ts := s.server.findSessionByGUID(guid); ts != nil && ts.player != nil {
		if s.isFriendlyToTarget(guid, ts) {
			return spellFailedTargetFriendly
		}
		return 0
	}
	s.server.motionMu.Lock()
	defer s.server.motionMu.Unlock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, guid)
	if motion == nil {
		return 0
	}
	caster := playerPos{Map: s.player.Map, InstanceID: s.player.InstanceID, X: s.player.X, Y: s.player.Y, Z: s.player.Z, GUID: s.playerGUID, Race: s.player.Race, Class: s.player.Class, Level: s.player.Level, FactionTemplate: s.server.raceFaction(s.player.Race), Reputations: playerReputationMap(s.player.Reputations), Sess: s}
	if s.server.isFriendlyFaction(motion.Faction, caster) {
		return spellFailedTargetFriendly
	}
	return 0
}

// wgCanFlyIn mirrors Battlefield::CanFlyIn (Battlefield.h:338): flight is
// forbidden while the battlefield is active.
func (wg *wgBattlegroundState) wgCanFlyIn() bool {
	if wg == nil {
		return true
	}
	wg.mu.Lock()
	defer wg.mu.Unlock()
	return !wg.IsActive
}

// checkFlyCast mirrors the SPELL_AURA_FLY /
// SPELL_AURA_MOD_INCREASE_MOUNTED_FLIGHT_SPEED leg of the CheckCast
// ApplyAuraName switch (Spell.cpp:6122-6135): a live player mounting a
// flying mount (or raising mounted flight speed) fails with
// SPELL_FAILED_NOT_HERE when the caster's area is flagged
// AREA_FLAG_NO_FLY_ZONE, or when the zone's battlefield is active
// (sBattlefieldMgr->GetBattlefieldToZoneId + Battlefield::CanFlyIn).
//
// The m_originalCaster arms are vacuous on the client path (the session is
// always the player caster), except IsAlive: dead and ghost casters skip
// the gate entirely — the C++ comment ("allow always ghost flight spells")
// is the IsAlive() arm. The missing-area arm is permissive: C++ runs the
// no-fly and battlefield tests only inside the LookupEntry success arm,
// so a missing AreaTable row skips both gates (terrain.go convention).
func (s *session) checkFlyCast(spell wotlk.Spell) uint8 {
	if s == nil || s.player == nil || s.server == nil || s.server.Data == nil {
		return 0
	}
	if !spellHasAura(spell, spellAuraFly) && !spellHasAura(spell, spellAuraModIncreaseMountedFlightSpeed) {
		return 0
	}
	if s.isDeadOrGhost() {
		return 0
	}
	area, found, areaErr := s.server.Data.Area(s.areaID)
	if areaErr != nil || !found {
		return 0
	}
	if area.Flags&areaFlagNoFlyZone != 0 {
		return spellFailedNotHere
	}
	s.server.wgMu.RLock()
	wg := s.server.wgState
	s.server.wgMu.RUnlock()
	if wg != nil && wg.ZoneID == s.player.Zone && !wg.wgCanFlyIn() {
		return spellFailedNotHere
	}
	return 0
}

// checkPeriodicManaLeechCast mirrors the SPELL_AURA_PERIODIC_MANA_LEECH
// leg of the CheckCast ApplyAuraName switch (Spell.cpp:6136-6147): a
// non-area mana-leech aura effect fails with
// SPELL_FAILED_BAD_IMPLICIT_TARGETS when the wire target carries no unit,
// and with SPELL_FAILED_BAD_TARGETS when the unit target's power type is not
// POWER_MANA (0, SharedDefines.h:295). The IsTargetingArea skip bridges
// SpellEffectInfo::IsTargetingArea (SpellInfo.cpp:380-383 — selection
// category AREA or CONE) via the enemy/friendly area target-type lists plus
// the friendly cone list. The caster-TYPEID_PLAYER arm is vacuous on the
// client path (session always a player), and handleCastSpell passes no cast
// item (verified at checkOpenLockCast — m_CastItem is always nil here), so
// the mana gate always applies once the effect and unit target resolve.
// Player targets read the shapeshift-aware playerPowerType (shapeshift.go);
// creature targets read the motion PowerType — reusing unitTargetPowerType,
// the same bridge as checkPowerBurnDrainCast. C++ gates only when
// GetUnitTarget() yields a unit, so unresolvable GUIDs skip the power check
// (pets have no power-type model). Returns the SPELL_FAILED_* result code, 0
// on success. Neither result carries extra WriteCastResultInfo params, so
// castFailedExtParams needs no case (verified Spell.cpp:3974-4160).
func (s *session) checkPeriodicManaLeechCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	matched := false
	for _, eff := range spell.Effects {
		if eff.Aura != spellAuraPeriodicManaLeech {
			continue
		}
		if isAreaEnemyTargetType(eff.ImplicitTargetA) || isAreaEnemyTargetType(eff.ImplicitTargetB) ||
			isFriendlyAreaTargetType(eff.ImplicitTargetA) || isFriendlyAreaTargetType(eff.ImplicitTargetB) ||
			isFriendlyConeTargetType(eff.ImplicitTargetA) || isFriendlyConeTargetType(eff.ImplicitTargetB) {
			continue
		}
		matched = true
		break
	}
	if !matched {
		return 0
	}
	if target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0 {
		return spellFailedBadImplicitTargets
	}
	targetPower, ok := s.unitTargetPowerType(target.UnitGUID)
	if !ok {
		return 0
	}
	if targetPower != 0 { // POWER_MANA
		return spellFailedBadTargets
	}
	return 0
}

// checkTradeSlotCast mirrors the trade-slot block of Spell::CheckCast
// (Spell.cpp:6164-6181). The m_CastItem arm is vacuous on the
// handleCastSpell path: book casts (CMSG_CAST_SPELL) never carry a cast
// item (Spell.cpp:584 — m_CastItem is set only by CastItemUseSpell), and
// the item-cast variant of the arm is already bridged at cast completion
// in finishSpellCast (spells.go:3429), which is the only CheckCast-time
// coverage item casts get (the CMSG_USE_ITEM path in items.go runs no
// CheckCast gates). The caster-TYPEID_PLAYER arm is vacuous (the session
// is always a player). Otherwise: no trade state (Player::GetTradeData)
// → SPELL_FAILED_NOT_TRADING; a wire item GUID other than the non-traded
// slot sentinel (TRADE_SLOT_NONTRADED = 6, TradeData.h:27 — the client
// sends the slot index as the item target GUID until
// UpdateTradeSlotItem rewrites it) → SPELL_FAILED_BAD_TARGETS; an
// enchant already deferred into the trade (TradeData::GetSpell) →
// SPELL_FAILED_ITEM_ALREADY_ENCHANTED. The !IsTriggered() guard is
// structural: handleCastSpell serves only client-initiated casts
// (server.go:1466) and triggered casts go through castSpellDirect, so
// the enchant-pending arm always applies here. Neither failure code
// carries extra WriteCastResultInfo params (verified
// Spell.cpp:3974-4160), so castFailedExtParams needs no case.
func (s *session) checkTradeSlotCast(target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil {
		return 0
	}
	if target.Flags&protocol.SpellTargetFlagTradeItem == 0 {
		return 0
	}
	if s.trade == nil {
		return spellFailedNotTrading
	}
	if target.ItemGUID != uint64(tradeSlotNonTraded) {
		return spellFailedBadTargets
	}
	if s.trade.SpellID != 0 {
		return spellFailedItemAlreadyEnchanted
	}
	return 0
}

// spellNeedsComboPoints mirrors SpellInfo::NeedsComboPoints
// (SpellInfo.cpp:1234-1237): SPELL_ATTR1_REQ_COMBO_POINTS1 |
// SPELL_ATTR1_REQ_COMBO_POINTS2 in AttributesEx (Spell.dbc field 5).
func spellNeedsComboPoints(spell wotlk.Spell) bool {
	return spell.AttributesEx&(spellAttr1ReqComboPoints1|spellAttr1ReqComboPoints2) != 0
}

// sessionComboPoints mirrors Unit::GetComboPoints (Unit.h:1579-1580).
// The GUID overload returns points only when the queried GUID matches the
// banked combo target; the no-arg overload (C++ GetComboPoints() with
// who=nullptr) returns whatever is banked regardless of target. A zero
// target GUID follows the null-who arm and returns the banked points.
func (s *session) sessionComboPoints(targetGUID uint64) uint8 {
	if s == nil || s.player == nil {
		return 0
	}
	if targetGUID == 0 {
		return s.comboPoints
	}
	if s.comboTargetGUID != targetGUID {
		return 0
	}
	return s.comboPoints
}

// sendComboPointsUpdate mirrors the player arm of Unit::SendComboPoints
// (Unit.cpp:10688-10697): SMSG_UPDATE_COMBO_POINTS (0x39D) carries the
// packed combo-target GUID and the point count. The npcbot/pet
// movingMe/owner arms are out of scope — the Go session is always the
// player.
func (s *session) sendComboPointsUpdate() {
	if s == nil || s.player == nil {
		return
	}
	packet := protocol.NewBuffer(packedGUIDSize(s.comboTargetGUID) + 1)
	packet.WritePackedGUID(s.comboTargetGUID)
	packet.WriteU8(s.comboPoints)
	_ = s.write(uint16(protocol.OpcodeSMSG_UPDATE_COMBO_POINTS), packet.Bytes(), true)
}

// addSessionComboPoints mirrors Unit::AddComboPoints (Unit.cpp:10655-10673):
// a new target resets the bank to count, the same target adds clamped to
// 0-5, and the client is notified. The m_comboPointHolders list has no Go
// bridge — Go keeps no reverse index from a creature GUID to the sessions
// banking points on it, so the holder cleanup on target death stays open.
func (s *session) addSessionComboPoints(targetGUID uint64, count int8) {
	if s == nil || s.player == nil || count == 0 {
		return
	}
	if targetGUID != 0 && targetGUID != s.comboTargetGUID {
		s.comboTargetGUID = targetGUID
		s.comboPoints = uint8(count)
	} else {
		total := int16(s.comboPoints) + int16(count)
		if total > 5 {
			total = 5
		}
		if total < 0 {
			total = 0
		}
		s.comboPoints = uint8(total)
	}
	s.sendComboPointsUpdate()
}

// clearSessionComboPoints mirrors Unit::ClearComboPoints
// (Unit.cpp:10674-10687): the bank empties and the client is notified. The
// SPELL_AURA_RETAIN_COMBO_POINTS removal has no Go bridge — the Go aura
// model tracks no retain-combo-points aura type.
func (s *session) clearSessionComboPoints() {
	if s == nil || s.player == nil || s.comboTargetGUID == 0 {
		return
	}
	s.comboPoints = 0
	s.sendComboPointsUpdate()
	s.comboTargetGUID = 0
}

// checkComboPointsCast mirrors the combo-point gate of Spell::CheckCast
// (Spell.cpp:6186-6209): a spell carrying REQ_COMBO_POINTS needs at least
// one banked combo point — against the explicit unit target when the spell
// needs one (SpellInfo::NeedsExplicitUnitTarget), banked points regardless
// of target otherwise — or the cast fails with SPELL_FAILED_NO_COMBO_POINTS
// (78). The m_caster->ToUnit() null arm is vacuous on the client path (the
// session is always a player) and the npcbot creature arm is out of scope.
// NO_COMBO_POINTS carries no extra WriteCastResultInfo params (verified
// Spell.cpp:3974-4160), so castFailedExtParams needs no case. Returns the
// SPELL_FAILED_* result code, 0 on success.
func (s *session) checkComboPointsCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil {
		return 0
	}
	if !spellNeedsComboPoints(spell) {
		return 0
	}
	var points uint8
	if spellNeedsExplicitUnitTarget(spell) {
		points = s.sessionComboPoints(target.UnitGUID)
	} else {
		points = s.sessionComboPoints(0)
	}
	if points == 0 {
		return spellFailedNoComboPoints
	}
	return 0
}

// immuneToMovementImpairmentAndLossControlMask mirrors
// IMMUNE_TO_MOVEMENT_IMPAIRMENT_AND_LOSS_CONTROL_MASK
// (SharedDefines.h:1393-1401), used by spellAllowedMechanicMask for the
// hardcoded SPELL_AURA_MECHANIC_IMMUNITY spell-id carve-outs.
const immuneToMovementImpairmentAndLossControlMask uint32 = (1 << 1) | (1 << 2) | (1 << 5) | (1 << 7) | (1 << 10) | (1 << 11) | (1 << 12) | (1 << 13) | (1 << 14) | (1 << 17) | (1 << 18) | (1 << 20) | (1 << 23) | (1 << 24) | (1 << 27) | (1 << 30)

// spellAllowedMechanicMask mirrors SpellInfo::GetAllowedMechanicMask
// (SpellInfo.cpp:3047-3049): the spell's own SPELL_AURA_MECHANIC_IMMUNITY
// (77) effects — including the hardcoded spell-id carve-outs
// (SpellInfo.cpp:2734-2758) — plus the SPELL_ATTR5_USABLE_WHILE_* bits
// (SpellInfo.cpp:2822-2856). The SPELL_AURA_MECHANIC_IMMUNITY_MASK (147)
// hardcoded spell-id table (SpellInfo.cpp:2592-2808) has no Go bridge yet —
// a documented gap; its entries are boss spells the client path never casts.
func spellAllowedMechanicMask(spell wotlk.Spell) uint32 {
	var mask uint32
	for _, eff := range spell.Effects {
		if eff.Effect != spellEffectApplyAura || eff.Aura != spellAuraMechanicImmunity {
			continue
		}
		switch spell.ID {
		case 42292, 59752: // PvP trinket, Every Man for Himself
			mask |= immuneToMovementImpairmentAndLossControlMask
		case 34471, 19574, 53490: // The Beast Within, Bestial Wrath, Bullheaded
			mask |= immuneToMovementImpairmentAndLossControlMask
		case 54508: // Demonic Empowerment
			mask |= (1 << 11) | (1 << 7) | (1 << 12) // MECHANIC_SNARE | MECHANIC_ROOT | MECHANIC_STUN
		default:
			if eff.MiscValue >= 1 {
				mask |= 1 << uint32(eff.MiscValue)
			}
		}
	}
	if spell.AttributesEx5&spellAttr5UsableWhileStunned != 0 {
		switch spell.ID {
		case 22812, 47585: // Barkskin, Dispersion
			mask |= (1 << 12) | (1 << 13) | (1 << 14) | (1 << 10) // MECHANIC_STUN | MECHANIC_FREEZE | MECHANIC_KNOCKOUT | MECHANIC_SLEEP
		case 49039: // Lichborne, don't allow normal stuns
		default:
			mask |= 1 << 12 // MECHANIC_STUN
		}
	}
	if spell.AttributesEx5&spellAttr5UsableWhileConfused != 0 {
		mask |= 1 << 2 // MECHANIC_DISORIENTED
	}
	if spell.AttributesEx5&spellAttr5UsableWhileFeared != 0 {
		switch spell.ID {
		case 22812, 47585: // Barkskin, Dispersion
			mask |= (1 << 5) | (1 << 24) // MECHANIC_FEAR | MECHANIC_HORROR
		default:
			mask |= 1 << 5 // MECHANIC_FEAR
		}
	}
	return mask
}

// spellCancelsAuraEffect mirrors SpellInfo::SpellCancelsAuraEffect
// (SpellInfo.cpp:3001-3044, "based on client Spell_C::CancelsAuraEffect"): a
// DISPEL_AURAS_ON_IMMUNITY spell cancels a preapplied aura effect when one of
// its APPLY_AURA immunity effects covers it. Go's wotlk.Spell.DispelType is
// the bridge for C++ SpellInfo::Dispel (SpellInfo.cpp:789).
func spellCancelsAuraEffect(spell, auraSpell wotlk.Spell, auraEffIndex int) bool {
	if spell.AttributesEx&spellAttr1DispelAurasOnImmunity == 0 {
		return false
	}
	if auraSpell.Attributes&spellAttr0UnaffectedByInvulnerability != 0 {
		return false
	}
	if auraEffIndex < 0 || auraEffIndex >= len(auraSpell.Effects) {
		return false
	}
	for _, eff := range spell.Effects {
		if eff.Effect != spellEffectApplyAura {
			continue
		}
		miscValue := uint32(eff.MiscValue)
		switch eff.Aura {
		case spellAuraStateImmunity:
			if miscValue != auraSpell.Effects[auraEffIndex].Aura {
				continue
			}
		case spellAuraSchoolImmunity, spellAuraModImmuneAuraApplySchool:
			if auraSpell.AttributesEx1&spellAttr2UnaffectedByAuraSchoolImmune != 0 || auraSpell.SchoolMask&miscValue == 0 {
				continue
			}
		case spellAuraDispelImmunity:
			if miscValue != auraSpell.DispelType {
				continue
			}
		case spellAuraMechanicImmunity:
			if miscValue != auraSpell.Mechanic {
				if miscValue != auraSpell.Effects[auraEffIndex].Mechanic {
					continue
				}
			}
		default:
			continue
		}
		return true
	}
	return false
}

// casterAuraEffectRef is one caster aura effect flattened per effect index,
// the self-caster view that Unit::GetAuraEffectsByType feeds the
// CheckCasterAuras helpers (Spell.cpp:6257+).
type casterAuraEffectRef struct {
	spellID  uint32
	effIndex int
}

// casterAuraEffectsByType collects the caster's own aura effects of the given
// type from activeAuras under castMu (the dispel.go locking pattern),
// resolving per-effect aura types from the spell row like
// targetAuraEffectsByType. Stopped auras are skipped; unresolvable spell
// rows fall back to the aura's single AuraType, matching the grouped model.
func (s *session) casterAuraEffectsByType(auraType uint32) []casterAuraEffectRef {
	if s == nil || s.player == nil {
		return nil
	}
	s.castMu.Lock()
	auras := make(map[uint32]*activeAura, len(s.activeAuras))
	for id, aura := range s.activeAuras {
		auras[id] = aura
	}
	s.castMu.Unlock()
	var out []casterAuraEffectRef
	for id, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		for i := 0; i < 3; i++ {
			if aura.EffectMask&(1<<uint(i)) == 0 {
				continue
			}
			t := aura.AuraType
			if sp, found, _ := s.server.Data.Spell(id); found && i < len(sp.Effects) {
				t = sp.Effects[i].Aura
			}
			if t != auraType {
				continue
			}
			out = append(out, casterAuraEffectRef{spellID: id, effIndex: i})
		}
	}
	return out
}

// checkSpellCancelsAuraEffect mirrors Spell::CheckSpellCancelsAuraEffect
// (Spell.cpp:6391-6415): every caster aura effect of auraType must be
// cancelled by the casting spell — an empty effect list passes outright. On
// the first non-cancelled effect *mechanic takes the effect's mechanic, else
// the aura's spell mechanic (Spell.cpp:6406-6410); unresolvable spell rows
// are permissive (terrain.go convention).
func (s *session) checkSpellCancelsAuraEffect(spell wotlk.Spell, auraType uint32, mechanic *uint32) bool {
	effects := s.casterAuraEffectsByType(auraType)
	if len(effects) == 0 {
		return true
	}
	for _, ref := range effects {
		auraSpell, found, _ := s.server.Data.Spell(ref.spellID)
		if !found {
			continue
		}
		if spellCancelsAuraEffect(spell, auraSpell, ref.effIndex) {
			continue
		}
		if mechanic != nil {
			*mechanic = auraSpell.Effects[ref.effIndex].Mechanic
			if *mechanic == 0 {
				*mechanic = auraSpell.Mechanic
			}
		}
		return false
	}
	return true
}

// checkCasterAurasMechanic mirrors the mechanicCheck lambda of
// Spell::CheckCasterAuras (Spell.cpp:6294-6334): a usable-while-CC spell is
// still blocked by CC aura effects whose whole-spell mechanic mask
// (GetAllEffectsMechanicMask, via spellMechanicMask) falls outside the
// casting spell's allowed mask. The first blocker wins; its mechanic (effect
// mechanic, else the aura's spell mechanic) feeds the packet param. The aura
// type maps to the failure code (the C++ ABORT() default is unreachable).
func (s *session) checkCasterAurasMechanic(spell wotlk.Spell, allowedMask, auraType uint32) (uint8, uint32) {
	var failure uint8
	switch auraType {
	case spellAuraModStun:
		failure = spellFailedStunned
	case spellAuraModFear:
		failure = spellFailedFleeing
	case spellAuraModConfuse:
		failure = spellFailedConfused
	default:
		return 0, 0
	}
	for _, ref := range s.casterAuraEffectsByType(auraType) {
		auraSpell, found, _ := s.server.Data.Spell(ref.spellID)
		if !found {
			continue
		}
		mechanicMask := spellMechanicMask(auraSpell)
		if mechanicMask == 0 || mechanicMask&allowedMask != 0 {
			continue
		}
		mechanic := auraSpell.Effects[ref.effIndex].Mechanic
		if mechanic == 0 {
			mechanic = auraSpell.Mechanic
		}
		return failure, mechanic
	}
	return 0, 0
}

// hasAuraMechanic reports whether any of the caster's live auras carries one
// of the mechanics in mask (Unit::HasAuraWithMechanic, Unit.cpp).
func (s *session) hasAuraMechanic(mask uint32) bool {
	if s == nil || s.player == nil {
		return false
	}
	s.castMu.Lock()
	auras := make(map[uint32]*activeAura, len(s.activeAuras))
	for id, aura := range s.activeAuras {
		auras[id] = aura
	}
	s.castMu.Unlock()
	for id, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		if sp, found, _ := s.server.Data.Spell(id); found && spellMechanicMask(sp)&mask != 0 {
			return true
		}
	}
	return false
}

// checkCasterAuras mirrors Spell::CheckCasterAuras (Spell.cpp:6257-6389),
// called from Spell::CheckCast (Spell.cpp:5509): spells immune to caster
// auras (ATTR6_IGNORE_CASTER_AURAS) pass outright; otherwise the caster's
// UNIT_FIELD_FLAGS state gates the cast — the ATTR5 usable-while-CC spells
// run the mechanic-mask check (with the Glyph of Pain Suppression carve-out:
// spell 33206 loses usable-while-stunned without aura 63248), and all other
// CC states require the spell to cancel the preventing aura effects.
// Returns the failure result and, when set, the mechanic id — a set mechanic
// converts the result to SPELL_FAILED_PREVENTED_BY_MECHANIC, which carries
// the mechanic as the extended packet param (WriteCastResultInfo,
// Spell.cpp:4102-4107).
//
// Client-path bridges: the session player is always the unit caster, so the
// ToUnit-null and original-caster arms are vacuous; the commented-out
// charmer block (Spell.cpp:6281-6291) has no Go model; the
// TRIGGERED_IGNORE_CASTER_AURAS gate is structural (handleCastSpell serves
// CMSG_CAST_SPELL only). The fleeing/confused arms are shadowed on this path:
// handleCastSpell's early pre-gate already rejects confused/fleeing casters,
// so the usable-while-feared/confused refinements never apply here — a
// pre-existing divergence, not introduced by this unit.
func (s *session) checkCasterAuras(spell wotlk.Spell) (uint8, uint32) {
	if spell.AttributesEx6&spellAttr6IgnoreCasterAuras != 0 {
		return 0, 0
	}
	usableWhileStunned := spell.AttributesEx5&spellAttr5UsableWhileStunned != 0
	usableWhileFeared := spell.AttributesEx5&spellAttr5UsableWhileFeared != 0
	usableWhileConfused := spell.AttributesEx5&spellAttr5UsableWhileConfused != 0
	if spell.ID == 33206 && !s.hasAura(63248) { // Pain Suppression without Glyph of Pain Suppression
		usableWhileStunned = false
	}
	allowedMask := spellAllowedMechanicMask(spell)
	var result uint8
	var mechanic uint32
	switch flags := s.player.UnitFlags; {
	case flags&unitFlagStunned != 0:
		if usableWhileStunned {
			result, mechanic = s.checkCasterAurasMechanic(spell, allowedMask, spellAuraModStun)
		} else if !(s.checkSpellCancelsAuraEffect(spell, spellAuraModStun, &mechanic) &&
			s.checkSpellCancelsAuraEffect(spell, spellAuraStrangulate, &mechanic)) {
			result = spellFailedStunned
		} else if spell.Mechanic&29 != 0 && s.hasAuraMechanic(1<<18) {
			// Immune-shield arm (Spell.cpp:6327-6329): C++ ANDs the mechanic
			// enum value itself against MECHANIC_IMMUNE_SHIELD (29); a banish
			// mechanic on the caster re-fails the cast.
			result = spellFailedStunned
		}
	case flags&unitFlagSilenced != 0 && spell.PreventionType == spellPreventionTypeSilence:
		if !(s.checkSpellCancelsAuraEffect(spell, spellAuraModSilence, &mechanic) &&
			s.checkSpellCancelsAuraEffect(spell, spellAuraModPacifySilence, &mechanic)) {
			result = spellFailedSilenced
		}
	case flags&unitFlagPacified != 0 && spell.PreventionType == spellPreventionTypePacify:
		if !(s.checkSpellCancelsAuraEffect(spell, spellAuraModPacify, &mechanic) &&
			s.checkSpellCancelsAuraEffect(spell, spellAuraModPacifySilence, &mechanic)) {
			result = spellFailedPacified
		}
	case flags&unitFlagFleeing != 0:
		if usableWhileFeared {
			result, mechanic = s.checkCasterAurasMechanic(spell, allowedMask, spellAuraModFear)
		} else if !s.checkSpellCancelsAuraEffect(spell, spellAuraModFear, &mechanic) {
			result = spellFailedFleeing
		}
	case flags&unitFlagConfused != 0:
		if usableWhileConfused {
			result, mechanic = s.checkCasterAurasMechanic(spell, allowedMask, spellAuraModConfuse)
		} else if !s.checkSpellCancelsAuraEffect(spell, spellAuraModConfuse, &mechanic) {
			result = spellFailedConfused
		}
	}
	if result != 0 && mechanic != 0 {
		return spellFailedPreventedByMechanic, mechanic
	}
	return result, mechanic
}

// checkAuraBouncedCast mirrors the AURA_BOUNCED recheck at the end of the
// per-effect loop in Spell::CheckCast (Spell.cpp:6155-6162): for a spell
// whose every effect is an aura effect — any non-aura effect sets
// nonAuraEffectMask (Spell.cpp:6022-6027) and disables the recheck for the
// whole spell — and which does not target an area
// (SpellInfo::IsTargetingArea, SpellInfo.cpp:1039-1045), each aura effect
// fails with SPELL_FAILED_AURA_BOUNCED when the unit target already
// carries a strictly more powerful aura of the same aura type under an
// EXCLUSIVE_HIGHEST spell-group stack rule
// (Unit::IsHighestExclusiveAuraEffect, Unit.cpp:14001-14036, with
// removeOtherAuraApplications=false at CheckCast). AURA_BOUNCED carries no
// extra WriteCastResultInfo params, so castFailedExtParams needs no case
// (verified Spell.cpp:3974-4160). Returns the SPELL_FAILED_* result code,
// 0 on success.
func (s *session) checkAuraBouncedCast(spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	if s == nil || s.player == nil || s.server == nil {
		return 0
	}
	var approximateMask, nonAuraMask uint8
	for i, eff := range spell.Effects {
		if i >= 8 {
			break
		}
		switch {
		case spellEffectIsAuraEffect(eff): // SpellEffectInfo::IsAura (SpellInfo.cpp:370-373)
			approximateMask |= 1 << uint(i)
		case eff.Effect != 0: // SpellEffectInfo::IsEffect (SpellInfo.cpp:360-363)
			nonAuraMask |= 1 << uint(i)
		}
	}
	if nonAuraMask != 0 {
		return 0
	}
	if target.Flags&protocol.SpellTargetFlagUnitWireMask == 0 || target.UnitGUID == 0 {
		return 0
	}
	if spellInfoTargetsArea(spell) {
		return 0
	}
	for i, eff := range spell.Effects {
		if i >= 8 {
			break
		}
		if approximateMask&(1<<uint(i)) == 0 {
			continue
		}
		if !s.isHighestExclusiveAuraEffect(spell, eff.Aura, s.spellEffectCheckCastValue(spell, eff, i), approximateMask, target.UnitGUID) {
			return spellFailedAuraBounced
		}
	}
	return 0
}

// spellInfoTargetsArea mirrors SpellInfo::IsTargetingArea
// (SpellInfo.cpp:1039-1045): any non-NONE effect whose implicit target A
// or B has AREA or CONE selection category, via the same target-type lists
// the periodic-mana-leech leg uses for SpellEffectInfo::IsTargetingArea.
func spellInfoTargetsArea(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if isAreaEnemyTargetType(eff.ImplicitTargetA) || isAreaEnemyTargetType(eff.ImplicitTargetB) ||
			isFriendlyAreaTargetType(eff.ImplicitTargetA) || isFriendlyAreaTargetType(eff.ImplicitTargetB) ||
			isFriendlyConeTargetType(eff.ImplicitTargetA) || isFriendlyConeTargetType(eff.ImplicitTargetB) {
			return true
		}
	}
	return false
}

// spellEffectCheckCastValue mirrors SpellEffectInfo::CalcValue
// (SpellInfo.cpp:402-460) as called from CheckCast (Spell.cpp:6159):
// m_spellValue->EffectBasePoints[i] is the DBC BasePoints on the client
// path (no custom base points), so CalcValueForLevel covers the
// base-points + caster-level scaling + die roll, and applySpellMod covers
// Unit::ApplyEffectModifiers (SPELLMOD_ALL_EFFECTS then
// SPELLMOD_EFFECT1+index). The combo-point term has no Go model
// (documented); the LEVEL_DAMAGE_CALCULATION scaling arm needs a
// non-player-controlled caster and is vacuous on the client path.
func (s *session) spellEffectCheckCastValue(spell wotlk.Spell, eff wotlk.SpellEffect, index int) int32 {
	value := eff.CalcValueForLevel(spell, uint32(s.player.Level))
	value = s.applySpellMod(spell, spellModAllEffects, value)
	value = s.applySpellMod(spell, spellModEffect1+uint8(index), value)
	return value
}

// auraBounceCandidate is one existing aura effect on the bounce target,
// flattened the way Unit::GetAuraEffectsByType feeds
// IsHighestExclusiveAuraEffect: the per-effect amount and the base aura's
// effect mask for the tie-break.
type auraBounceCandidate struct {
	spellID    uint32
	amount     int32
	effectMask uint8
}

// targetAuraEffectsByType collects the target unit's existing aura effects
// of the given aura type: player targets (including self-casts) from the
// session's activeAuras under its castMu, creature and pet targets from
// the server's activeCreatureAuras under auraMu (the dispel.go locking
// pattern). Per-effect aura types come from the aura's spell row, falling
// back to the aura's single AuraType when the row is missing (the grouped
// model documented in commands_list.go). Unresolvable GUIDs yield no
// candidates — C++ only gates when GetUnitTarget() yields a unit.
func (s *session) targetAuraEffectsByType(targetGUID uint64, auraType uint32) []auraBounceCandidate {
	var auras map[uint32]*activeAura
	if ts := s.server.findSessionByGUID(targetGUID); ts != nil && ts.player != nil {
		ts.castMu.Lock()
		auras = make(map[uint32]*activeAura, len(ts.activeAuras))
		for id, aura := range ts.activeAuras {
			auras[id] = aura
		}
		ts.castMu.Unlock()
	} else {
		s.server.auraMu.Lock()
		auras = s.server.activeCreatureAuras[creatureAuraKeyForPlayer(*s.player, targetGUID)]
		s.server.auraMu.Unlock()
	}
	var out []auraBounceCandidate
	for id, aura := range auras {
		if aura == nil || aura.Stopped {
			continue
		}
		for i := 0; i < 3; i++ {
			if aura.EffectMask&(1<<uint(i)) == 0 {
				continue
			}
			t := aura.AuraType
			if sp, found, _ := s.server.Data.Spell(id); found && i < len(sp.Effects) {
				t = sp.Effects[i].Aura
			}
			if t != auraType {
				continue
			}
			out = append(out, auraBounceCandidate{spellID: id, amount: aura.Amounts[i], effectMask: aura.EffectMask})
		}
	}
	return out
}

// isHighestExclusiveAuraEffect mirrors Unit::IsHighestExclusiveAuraEffect
// (Unit.cpp:14001-14036) with removeOtherAuraApplications=false, the
// CheckCast call shape (Spell.cpp:6156-6160): the new aura effect is
// "highest" unless an existing same-type aura effect on the target shares
// an EXCLUSIVE_HIGHEST spell group with the new spell and is strictly
// more powerful — absolute amount first, effect-mask bit count as the
// tie-break. The diff>0 removal arms are dead at CheckCast (nothing is
// removed there), so only the diff<0 bounced verdict is reported. Unlike
// exclusiveHighestVerdict (the apply-path variant), same-spell auras are
// not skipped — C++ compares them too.
func (s *session) isHighestExclusiveAuraEffect(spell wotlk.Spell, auraType uint32, effectAmount int32, auraEffectMask uint8, targetGUID uint64) bool {
	existing := s.targetAuraEffectsByType(targetGUID, auraType)
	if len(existing) == 0 {
		return true
	}
	newFirst := s.server.spellFirstRank(spell.ID)
	newAbs := absAuraAmount(effectAmount)
	newBits := int64(popcount8(auraEffectMask))
	for _, e := range existing {
		if s.server.spellGroupStackRule(newFirst, s.server.spellFirstRank(e.spellID)) != spellGroupStackRuleExclusiveHighest {
			continue
		}
		diff := newAbs - absAuraAmount(e.amount)
		if diff == 0 {
			diff = newBits - int64(popcount8(e.effectMask))
		}
		if diff < 0 {
			return false
		}
	}
	return true
}

// inSameGroupAs mirrors Player::IsInSameRaidWith (Player.cpp:2543-2546):
// the two sessions share a live non-zero group.
func (s *session) inSameGroupAs(other *session) bool {
	if s == nil || other == nil {
		return false
	}
	return s.groupID != 0 && s.groupID == other.groupID
}

// findInstanceBind returns the bind row for the given map and difficulty, or
// nil — the Go equivalent of Player::GetBoundInstance(map, difficulty).
func findInstanceBind(binds []instanceBindRow, mapID uint32, difficulty uint8) *instanceBindRow {
	for i := range binds {
		if binds[i].mapID == mapID && binds[i].difficulty == difficulty {
			return &binds[i]
		}
	}
	return nil
}

// creatureTameable mirrors CreatureTemplate::IsTameable
// (CreatureData.h:230-237): a beast with a set family and the tameable-pet
// type flag; exotic pets need canTameExotic (Player::CanTameExoticPets,
// Player.h:1826 — GM or SPELL_AURA_ALLOW_TAME_PET_TYPE).
func creatureTameable(ctype, family, typeFlags int64, canTameExotic bool) bool {
	if ctype != creatureTypeBeast || family == creatureFamilyNone || typeFlags&creatureTypeFlagTameablePet == 0 {
		return false
	}
	return canTameExotic || typeFlags&creatureTypeFlagExoticPet == 0
}

// canTameExoticPets mirrors Player::CanTameExoticPets (Player.h:1826):
// game-master state or an active SPELL_AURA_ALLOW_TAME_PET_TYPE aura.
func (s *session) canTameExoticPets() bool {
	if s == nil || s.player == nil {
		return false
	}
	if s.player.ExtraFlags&playerExtraGMOn != 0 || s.player.PlayerFlags&playerFlagGM != 0 {
		return true
	}
	return s.hasAuraType(spellAuraAllowTamePetType)
}

// sendTameFailure mirrors Player::SendTameFailure (Player.cpp:3084-3089):
// SMSG_PET_TAME_FAILURE carrying the single-byte PetTameFailure reason.
func (s *session) sendTameFailure(reason uint8) {
	if s == nil {
		return
	}
	buf := protocol.NewBuffer(1)
	buf.WriteU8(reason)
	_ = s.write(uint16(protocol.OpcodeSMSG_PET_TAME_FAILURE), buf.Bytes(), true)
}

// spellDiminishingBounced mirrors the diminishing-returns recheck leg of
// Spell::_cast (Spell.cpp:3374-3400) and Unit::HasStrongerAuraWithDR
// (Unit.cpp:4744-4762): after the cast bar completes, a crowd-control spell
// whose DR-adjusted duration would land shorter than an already-active aura
// of the same DR group on the target fails with SPELL_FAILED_AURA_BOUNCED.
// Only player targets bridge the check: Go's diminishing state is
// session-scoped, with no per-creature Unit::m_Diminishings bridge, so
// creature targets skip the recheck (documented gap).
func (s *session) spellDiminishingBounced(spell wotlk.Spell, targetGUID uint64) bool {
	if s == nil || s.server == nil || s.server.Data == nil || targetGUID == 0 {
		return false
	}
	ownedAura := false
	for _, eff := range spell.Effects {
		if spellEffectIsUnitOwnedAuraEffect(eff) {
			ownedAura = true
			break
		}
	}
	if !ownedAura {
		return false
	}
	// finishSpellCast serves player-initiated casts (m_triggeredByAuraSpell
	// is null), so the non-triggered DR group applies; Go's
	// getDiminishingReturnsGroup mirrors diminishingGroupCompute(false)
	// (SpellInfo.cpp:2242) — the triggered variant differs only for the
	// mechanic-STUN/ROOT fallbacks (SpellInfo.cpp:2424/2428).
	group := getDiminishingReturnsGroup(spell.ID, spell.Mechanic)
	if group == DiminishingNone {
		return false
	}
	// DiminishingReturnsType gate (SpellInfo.cpp:2435-2449): DRTYPE_ALL
	// groups recheck on every target, DRTYPE_PLAYER only on DR-affected
	// targets (Unit::IsAffectedByDiminishingReturns, Unit.h:780) — a live
	// session is always a player, so the gate passes for player targets.
	if t := diminishingGroupType(group); t != diminishingTypeAll && t != diminishingTypePlayer {
		return false
	}
	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil {
		return false
	}
	// SpellInfo::GetMaxDuration (SpellInfo.cpp:3084-3089): the DBC
	// MaxDuration column; -1 (infinite) and 0 (no duration entry) never
	// bounce (Unit.cpp:4757 requires newDuration > 0).
	maxDuration, found, err := s.server.Data.SpellMaxDuration(spell.DurationIndex)
	if err != nil || !found || maxDuration <= 0 {
		return false
	}
	for _, aura := range targetSess.loadedAuras() {
		if aura == nil {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		if getDiminishingReturnsGroup(aura.SpellID, auraSpell.Mechanic) != group {
			continue
		}
		// Aura::GetDuration is the live remaining duration
		// (player_aura_save.go:305-318 pattern).
		existing := aura.RemainingMs
		if !aura.DurationUpdatedAt.IsZero() {
			if elapsed := time.Since(aura.DurationUpdatedAt).Milliseconds(); elapsed > 0 {
				if uint64(elapsed) < uint64(existing) {
					existing -= uint32(elapsed)
				} else {
					existing = 0
				}
			}
		}
		// Unit::ApplyDiminishingToDuration (Unit.cpp:9036-9099) against the
		// target's own DR level: the 10s PvP cap (caster is the player,
		// target is DR-affected) then the level modifier. Run on the
		// target session so its GetDiminishing level applies; the taunt
		// special-case mods only apply to creature targets (Unit.cpp:9069).
		if _, newDuration, ok := targetSess.applyDiminishingToDuration(spell.ID, spell.Mechanic, uint32(maxDuration), true); ok && newDuration > 0 && newDuration < existing {
			return true
		}
	}
	return false
}

// spellHitTrigger mirrors Spell::TriggerOnHitEntry (Spell.h): a
// SPELL_AURA_ADD_TARGET_TRIGGER snapshot taken at cast completion.
type spellHitTrigger struct {
	TriggerSpellID       uint32
	TriggeredByAuraSpell uint32
	Chance               int32
}

// prepareHitTriggerSpells mirrors Spell::PrepareTriggersExecutedOnHit
// (Spell.cpp:8176-8206): it snapshots the caster's SPELL_AURA_ADD_TARGET_TRIGGER
// aura effects present at cast completion, so triggered auras gained mid-cast
// cannot affect the caster and proc chance uses the combo-point-independent
// base amount. The snapshot is stored on the completed cast for the on-hit
// trigger consumer.
func (s *session) prepareHitTriggerSpells(spell wotlk.Spell) []spellHitTrigger {
	if s == nil || s.server == nil || s.server.Data == nil {
		return nil
	}
	var triggers []spellHitTrigger
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != spellAuraAddTargetTrigger || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			// AuraEffect::IsAffectedOnSpell (SpellAuraEffects.cpp:848).
			if !spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				continue
			}
			if effect.TriggerSpell == 0 {
				continue
			}
			if _, found, err := s.server.Data.Spell(effect.TriggerSpell); err != nil || !found {
				continue
			}
			// Proc chance is stored in the effect amount; C++ runs it
			// through Unit::CalculateSpellDamage (done mods) before
			// multiplying by the stack amount. Go has no CalculateSpellDamage
			// bridge for aura proc chances, so the base amount is used
			// directly (documented delta).
			chance := aura.BaseAmounts[index]
			if chance == 0 {
				chance = effect.BasePoints + 1
			}
			chance *= int32(aura.StackAmount)
			triggers = append(triggers, spellHitTrigger{
				TriggerSpellID:       effect.TriggerSpell,
				TriggeredByAuraSpell: aura.SpellID,
				Chance:               chance,
			})
		}
	}
	return triggers
}

// canExecuteHitTriggers mirrors Spell::CanExecuteTriggersOnHit
// (Spell.cpp:8164-8174): an effect bit in the hit mask fires the trigger
// unless the triggering aura carries SPELL_ATTR4_PROC_ONLY_ON_CASTER
// (SharedDefines.h:561), in which case only effects whose implicit target A
// is TARGET_UNIT_CASTER (SharedDefines.h:1442) qualify.
func (s *session) canExecuteHitTriggers(spell wotlk.Spell, effMask uint8, triggeredByAuraSpell uint32) bool {
	onlyOnCaster := false
	if triggeredByAuraSpell != 0 && s != nil && s.server != nil && s.server.Data != nil {
		if auraSpell, found, err := s.server.Data.Spell(triggeredByAuraSpell); err == nil && found {
			onlyOnCaster = auraSpell.AttributesEx4&spellAttr4ProcOnlyOnCaster != 0
		}
	}
	for i, eff := range spell.Effects {
		if effMask&(1<<uint(i)) == 0 {
			continue
		}
		if !onlyOnCaster || eff.ImplicitTargetA == targetUnitCaster {
			return true
		}
	}
	return false
}

// fireHitTriggerSpells mirrors Spell::DoTriggersOnSpellHit
// (Spell.cpp:2913-2957): the per-hit consumer of the
// PrepareTriggersExecutedOnHit snapshot. C++ calls it inside
// DoTargetSpellHit after damage/healing is dealt (Spell.cpp:2650-2652);
// Go calls it per unit hit target after the effects loop. Two legs:
//   - ADD_TARGET_TRIGGER snapshot: per trigger, the CanExecuteTriggersOnHit
//     effMask gate plus the roll_chance_i chance fires
//     m_caster->CastSpell(unit, triggeredSpell, true) on the hit target; a
//     triggered aura with no duration (-1) inherits the remaining duration
//     of the cast spell's own aura on the target from the caster.
//   - spell_linked_spell hit rows (type 1, id + SPELL_LINK_HIT,
//     SpellMgr.h:106): negative ids RemoveAurasDueToSpell on the hit
//     target, positive ids are cast by the hit target on itself with the
//     caster as original caster.
//
// effMask is the mask of the spell's non-zero effects: Go applies every
// non-zero effect to every hit target, so there is no per-target effect
// filtering like C++'s per-target EffectMask.
func (s *session) fireHitTriggerSpells(ctx context.Context, spell wotlk.Spell, spellID uint32, effMask uint8, triggers []spellHitTrigger, targetGUID uint64) {
	if s == nil || s.server == nil || targetGUID == 0 {
		return
	}
	// DoTargetSpellHit runs per unit target; GO guids flow through
	// hitTargets like C++'s AddGOTarget but are never hit targets.
	if high := uint16(targetGUID >> 48); high != 0 && high != 0xF130 {
		return
	}
	for _, t := range triggers {
		if !s.canExecuteHitTriggers(spell, effMask, t.TriggeredByAuraSpell) {
			continue
		}
		// roll_chance_i(chance): urand(0, 99) < chance.
		if rand.Float64()*100 >= float64(t.Chance) {
			continue
		}
		// m_caster->CastSpell(unit, triggeredSpell->Id, true): triggered
		// cast by the caster on the hit target.
		s.castSpellDirect(ctx, t.TriggerSpellID, targetGUID)
		s.inheritHitTriggerAuraDuration(ctx, spellID, t.TriggerSpellID, targetGUID)
	}
	s.fireSpellLinkedHitTriggers(ctx, spellID, targetGUID)
}

// inheritHitTriggerAuraDuration mirrors the no-duration leg of
// Spell::DoTriggersOnSpellHit (Spell.cpp:2935-2949): SPELL_AURA_ADD_TARGET_TRIGGER
// auras must not trigger auras without duration, so when the triggered
// spell's raw duration is -1 (SpellInfo::GetDuration, SpellInfo.cpp:3077)
// the triggered aura — applied to the hit target by the caster — inherits
// the remaining duration of the cast spell's own aura on that target from
// the caster. Aura::GetDuration is the live remaining duration
// (player_aura_save.go:305-318 pattern).
func (s *session) inheritHitTriggerAuraDuration(ctx context.Context, spellID, triggeredSpellID uint32, targetGUID uint64) {
	if s == nil || s.server == nil || s.server.Data == nil {
		return
	}
	duration, found, err := s.server.Data.SpellDurationBase(triggeredSpellID)
	if err != nil || !found || duration != -1 {
		return
	}
	casterGUID := s.playerGUID
	remainingOf := func(remainingMs uint32, updatedAt time.Time) uint32 {
		remaining := int64(remainingMs)
		if !updatedAt.IsZero() {
			if elapsed := time.Since(updatedAt).Milliseconds(); elapsed > 0 {
				if uint64(elapsed) < uint64(remaining) {
					remaining -= elapsed
				} else {
					remaining = 0
				}
			}
		}
		if remaining < 0 {
			remaining = 0
		}
		return uint32(remaining)
	}
	// unit->GetAura(spellId, casterGUID) on the hit target: player targets
	// carry their auras on their session.
	if ts := s.server.findSessionByGUID(targetGUID); ts != nil {
		var base, triggered *activeAura
		for _, aura := range ts.loadedAuras() {
			if aura == nil || aura.Stopped || aura.CasterGUID != casterGUID {
				continue
			}
			switch aura.SpellID {
			case spellID:
				base = aura
			case triggeredSpellID:
				triggered = aura
			}
		}
		if base == nil || triggered == nil {
			return
		}
		remaining := remainingOf(base.RemainingMs, base.DurationUpdatedAt)
		triggered.DurationMs = remaining
		triggered.RemainingMs = remaining
		triggered.DurationUpdatedAt = time.Now()
		// Aura::SetDuration pushes the client update
		// (SpellAuras.cpp:902); the triggered aura was applied as
		// permanent, so refresh the client's duration here.
		ts.sendAuraUpdate(triggered.Slot, triggeredSpellID, false, triggered.Positive, remaining, remaining)
		return
	}
	// Creature hit target: the server-side creature aura records.
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok {
		return
	}
	key := creatureAuraKeyForTarget(target)
	s.server.auraMu.Lock()
	defer s.server.auraMu.Unlock()
	auras := s.server.activeCreatureAuras[key]
	if auras == nil {
		return
	}
	base, ok := auras[spellID]
	if !ok || base == nil || base.Stopped || base.CasterGUID != casterGUID {
		return
	}
	triggered, ok := auras[triggeredSpellID]
	if !ok || triggered == nil || triggered.Stopped || triggered.CasterGUID != casterGUID {
		return
	}
	remaining := remainingOf(base.RemainingMs, base.DurationUpdatedAt)
	triggered.DurationMs = remaining
	triggered.RemainingMs = remaining
	triggered.DurationUpdatedAt = time.Now()
}

// fireSpellLinkedHitTriggers mirrors the spell_linked tail of
// Spell::DoTriggersOnSpellHit (Spell.cpp:2951-2957): rows of
// spell_linked_spell with type 1 (id + SPELL_LINK_HIT, SpellMgr.h:106)
// fire per hit target — negative ids RemoveAurasDueToSpell on the hit
// target, positive ids are cast by the hit target on itself with the caster
// as original caster. (Spell.cpp carries a @todo to remove this table; Go
// mirrors the cast leg in fireSpellLinkedTriggers, so the hit leg is
// mirrored here for the same reason.)
func (s *session) fireSpellLinkedHitTriggers(ctx context.Context, spellID uint32, targetGUID uint64) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT spell_effect FROM spell_linked_spell WHERE spell_trigger = ? AND type = 1", int32(spellID))
	if err != nil {
		if !missingTable(err) {
			s.debug("spell_linked_spell hit query failed", "spell", spellID, "err", err)
		}
		return
	}
	var effects []int32
	for rows.Next() {
		var effect int32
		if err := rows.Scan(&effect); err == nil {
			effects = append(effects, effect)
		}
	}
	rows.Close()
	for _, id := range effects {
		if id == 0 {
			continue
		}
		if id < 0 {
			// Unit::RemoveAurasDueToSpell on the hit target.
			linkedSpellID := uint32(-id)
			if ts := s.server.findSessionByGUID(targetGUID); ts != nil {
				ts.removeAura(linkedSpellID)
				continue
			}
			if target, ok := s.getCombatTarget(ctx, targetGUID); ok {
				key := creatureAuraKeyForTarget(target)
				s.server.auraMu.Lock()
				slot, found := uint8(0), false
				if auras := s.server.activeCreatureAuras[key]; auras != nil {
					if aura, ok := auras[linkedSpellID]; ok && aura != nil && !aura.Stopped {
						slot, found = aura.Slot, true
					}
				}
				s.server.auraMu.Unlock()
				if found {
					s.expireCreatureAura(key, linkedSpellID, slot)
				}
			}
			continue
		}
		// unit->CastSpell(unit, id, casterGUID): the hit target casts on
		// itself. Player targets cast through their own session; Go has no
		// creature caster path, so the creature leg is a documented gap.
		if ts := s.server.findSessionByGUID(targetGUID); ts != nil {
			ts.castSpellDirect(ctx, uint32(id), targetGUID)
		}
	}
}

// hasConsumeNoAmmoAura mirrors the HandleLaunchPhase ammo exemption
// (Spell.cpp:7705): Player::HasAuraTypeWithAffectMask(
// SPELL_AURA_ABILITY_CONSUME_NO_AMMO, spell) — the caster's aura effects of
// that type whose family mask covers the cast spell suppress TakeAmmo.
func (s *session) hasConsumeNoAmmoAura(spell wotlk.Spell) bool {
	if s == nil || s.server == nil || s.server.Data == nil {
		return false
	}
	for _, aura := range s.loadedAuras() {
		if aura == nil || aura.Stopped {
			continue
		}
		auraSpell, found, err := s.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if effect.Aura != spellAuraAbilityConsumeNoAmmo || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			// AuraEffect::IsAffectedOnSpell (SpellAuraEffects.cpp:848).
			if spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				return true
			}
		}
	}
	return false
}

func (s *session) finishSpellCast(ctx context.Context, castID uint8, spellID uint32, spell wotlk.Spell, target protocol.SpellTargetData, castItemGUID uint64, castItemEntry uint32) {
	if s.player == nil {
		return
	}
	// Spell::_cast (Spell.cpp:3323): the cast holds the spellmod taking
	// window for its whole execution; the deferred end mirrors the _cast
	// tail and failure exits (Spell.cpp:3418, 3519).
	s.beginSpellModTaking()
	defer s.endSpellModTaking()
	var completedCast *activeCastState
	s.castMu.Lock()
	if s.activeCast != nil && s.activeCast.CastID == castID && s.activeCast.SpellID == spellID {
		if s.activeCast.Cancelled {
			s.castMu.Unlock()
			return
		}
		completedCast = s.activeCast
	}
	s.castMu.Unlock()
	if completedCast != nil {
		defer func() {
			s.castMu.Lock()
			if s.activeCast == completedCast {
				s.activeCast = nil
			}
			s.castMu.Unlock()
		}()
	}
	if s.isDeadOrGhost() {
		canResurrect := false
		for _, effect := range spell.Effects {
			if effect.Effect == spellEffectResurrectNew {
				canResurrect = true
				break
			}
		}
		if !canResurrect {
			return
		}
	}

	// Spell::_cast (Spell.cpp:3293-3304): as of 3.0.2 the caster's pets begin
	// attacking the owner's target immediately when the owner casts a harmful
	// spell. C++ runs this before the CheckCast revalidation below, so it
	// fires even when the cast then fails; target.UnitGUID is the explicit
	// unit target straight from the client packet (m_targets.GetUnitTarget()),
	// before any target selection.
	if spell.DefenseType != spellDamageClassNone && target.UnitGUID != 0 && target.UnitGUID != s.playerGUID && s.server != nil {
		s.server.triggerPetOwnerAttacked(s.player.Map, s.player.InstanceID, s.playerGUID, target.UnitGUID)
	}

	// Spell::_cast (Spell.cpp:3335) re-runs CheckCast(false) when the cast timer
	// finishes; power drained mid-cast must fail the cast, not clamp to zero.
	pType := spell.PowerType
	cost := s.calculateSpellPowerCost(spell)
	if pType < 7 && cost > 0 && s.player.Powers[pType] < cost {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 85), true) // SPELL_FAILED_NO_POWER = 85
		return
	}
	// Spell::_cast re-runs CheckCast at completion; runes spent mid-cast
	// must fail the cast too (Spell::CheckPower rune check, Spell.cpp:6665).
	if spell.PowerType == 5 && !s.checkRuneCost(spell, time.Now().UnixMilli()) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 85), true) // SPELL_FAILED_NO_POWER = 85
		return
	}

	// Spell::_cast revalidates CheckCast at completion: the target may have
	// moved during the cast bar.
	if target.UnitGUID != 0 {
		if failCode := s.validateSpellRange(ctx, spellID, spell, target.UnitGUID); failCode != 0 {
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, failCode), true)
			s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "range", "code", failCode)
			return
		}
		// CheckCast also revalidates line of sight at completion.
		if tgt, ok := s.getCombatTarget(ctx, target.UnitGUID); ok && s.server != nil {
			if !s.server.hasLineOfSight(s.player.Map, s.player.X, s.player.Y, s.player.Z, tgt.X, tgt.Y, tgt.Z) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 47), true) // SPELL_FAILED_LINE_OF_SIGHT = 47
				s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "line of sight")
				return
			}
			// Unit targets that died during the cast bar fail with
			// SPELL_FAILED_TARGETS_DEAD (SpellInfo::CheckTarget, SpellInfo.cpp:1715)
			// unless the spell allows dead targets (SpellInfo::IsAllowingDeadTarget,
			// SpellInfo.cpp:1177) or can resurrect.
			if tgt.Health == 0 && target.UnitGUID != s.playerGUID {
				canResurrect := false
				for _, effect := range spell.Effects {
					if effect.Effect == spellEffectResurrectNew {
						canResurrect = true
						break
					}
				}
				if !canResurrect && !spellAllowsDeadTarget(spell) {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 109), true)
					s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "target dead")
					return
				}
			}
		}
	}

	// Spell::CheckCast trade-slot gate (Spell.cpp:6167-6171): a spell cast
	// from an item (enchanting vellum via CMSG_USE_ITEM) cannot target the
	// trade window's non-traded slot — SPELL_FAILED_ITEM_ENCHANT_TRADE_WINDOW —
	// and is never deferred into the trade data. The gate runs before the
	// deferral below, matching _cast order (CheckCast(false) precedes the
	// TARGET_FLAG_TRADE_ITEM deferral, Spell.cpp:3335-3372); it is not gated
	// on trade state because C++ checks m_CastItem before the NOT_TRADING
	// terms. Book casts (CMSG_CAST_SPELL) always pass 0 here (Spell.cpp:584:
	// m_CastItem is set only by CastItemUseSpell), so only item casts trip it.
	if target.Flags&protocol.SpellTargetFlagTradeItem != 0 && castItemGUID != 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedItemEnchantTradeWindow), true)
		s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "item enchant trade window")
		return
	}

	// Spell::_cast (Spell.cpp:3357-3372): a cast-bar-completed spell targeting
	// the trade window's non-traded slot is deferred into the trade data while
	// the trade is not in its accept process — the spell fires when the trade
	// executes (TradeHandler.cpp:364-438). m_CastItem is null for CMSG_CAST_SPELL
	// book casts (Spell.cpp:584, set only by CastItemUseSpell / UpdatePointers),
	// so the stored cast-item GUID is 0. The deferred activeCast cleanup at the
	// top of this function mirrors cleanupSpell(SPELL_FAILED_DONT_REPORT)'s
	// silent drop; nothing is sent to the client here.
	if target.Flags&protocol.SpellTargetFlagTradeItem != 0 && s.trade != nil && !s.trade.InAcceptProcess {
		s.setTradeSpell(spellID, 0)
		s.debug("spell cast deferred to trade", "account", s.accountName, "spell", spellID)
		return
	}

	// Spell::_cast (Spell.cpp:3374-3400): the diminishing-returns recheck
	// runs again after the cast bar completes — a crowd-control spell whose
	// DR-adjusted duration would land shorter than an already-active aura of
	// the same DR group on the target bounces with SPELL_FAILED_AURA_BOUNCED
	// (Unit::HasStrongerAuraWithDR, Unit.cpp:4744-4762). target.UnitGUID is
	// the explicit unit target from the packet (m_targets.GetUnitTarget()),
	// matching _cast order before SelectSpellTargets.
	if target.UnitGUID != 0 && s.spellDiminishingBounced(spell, target.UnitGUID) {
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedAuraBounced), true)
		s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "diminishing returns bounce")
		return
	}

	// Spell::_cast (Spell.cpp:3430): PrepareTriggersExecutedOnHit snapshots
	// the caster's SPELL_AURA_ADD_TARGET_TRIGGER auras after the completion
	// rechecks and before target selection. The on-hit consumer
	// (Spell::DoTriggersOnSpellHit, Spell.cpp:2913 — per-hit trigger cast
	// with the CanExecuteTriggersOnHit effMask gate and the no-duration aura
	// duration copy, plus the spell_linked hit rows) is fireHitTriggerSpells,
	// called per unit hit target at the end of the effects loop below; the
	// snapshot is stored on the completed cast for it.
	if completedCast != nil {
		completedCast.HitTriggers = s.prepareHitTriggerSpells(spell)
	}

	// Spell::_cast (Spell.cpp:3431-3470) remaining legs:
	//   - CallScriptOnCastHandlers: no SpellScript bridge in Go, so there is
	//     no consumer to call (no-op, like the other script legs).
	//   - UpdateTradeSlotItem: the trade-slot sentinel (TRADE_SLOT_NONTRADED,
	//     set server-side by SetTradeItemTarget) only exists for the deferred
	//     trade spell; Go resolves the trader's non-traded item directly in
	//     applyDeferredTradeEnchant (trade.go), and client packets carry the
	//     real item GUID, so there is no sentinel to rewrite.
	//   - HandleLaunchPhase: the LAUNCH effect modes, launch-time combat
	//     engage, and wand/thrown TakeAmmo legs have no Go bridge; the
	//     ammo leg is bridged at SendSpellGo below, and the delayed leg
	//     is the projectile travel delay before effect execution (the
	//     spell.Speed leg further below).
	//   - ReleaseSpellFocus: creature casters only; the caster here is always
	//     a player (s.player == nil returns at the top).
	//   - TakePower/TakeReagents (Spell.cpp:3449-3457): the cost gate skips
	//     both under TRIGGERED_IGNORE_POWER_AND_REAGENT_COST, but a
	//     triggered cast whose item target is not owned by the caster still
	//     takes reagents. finishSpellCast only serves non-triggered casts
	//     (C++ TRIGGERED_NONE), so power (deducted below before SendSpellGo,
	//     matching "Powers have to be taken before SendSpellGo") and
	//     reagents (takeSpellReagents) always run here. Triggered casts never
	//     enter finishSpellCast — the flag exemption is structural (see
	//     castSpellDirectWithOverrides). The non-owned item-target arm has no
	//     bridge: Go parses the wire item GUID but has no
	//     m_targets.GetItemTarget() model in the cast flow (unit targets
	//     only), so a triggered cast can never carry a non-owned item target.
	//
	// Spell::_cast (Spell.cpp:3438-3444): a player cast from an item
	// (CMSG_USE_ITEM, m_CastItem) starts the item-use timed achievement and
	// updates ACHIEVEMENT_CRITERIA_TYPE_USE_ITEM with the cast item's entry,
	// gated on TRIGGERED_IGNORE_CAST_ITEM not being set (SpellDefines.h:137
	// — "Will not take away cast item or update related achievement
	// criteria"). finishSpellCast's only non-zero cast-item callers are
	// genuine CMSG_USE_ITEM casts (items.go), so castItemGUID != 0 is the Go
	// model of that gate. m_CastItem is a held pointer in C++; the entry
	// rides the call because consumables are decremented at cast start and
	// the item_instance row may be gone by completion.
	if castItemGUID != 0 && castItemEntry != 0 {
		s.startTimedAchievement(timedTypeItem, castItemEntry)
		s.updateAchievementCriteria(criteriaTypeUseItem, castItemEntry, 1)
	}

	// Spell::SelectImplicitTargetDestTargets (Spell.cpp:1433) and
	// Spell::SelectImplicitDestDestTargets (Spell.cpp:1464): resolve the
	// spell destination from target-relative / dest-relative implicit
	// targets once, before the area selection and persistent-area read
	// sites below consume it.
	target, destOK := s.resolveImplicitSpellDestination(ctx, spell, spellID, target)
	if !destOK {
		// Spell.cpp:1111: no nearby entry object found ->
		// SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993).
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 11), true)
		s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "no nearby entry object")
		return
	}

	hitTargets := make([]uint64, 0, 1)
	// Spell::CheckCast routes the explicit unit target through
	// SpellInfo::CheckExplicitTarget (Spell.cpp:5365, SpellInfo.cpp:1799),
	// which applies the Unit::IsValidAttackTarget/IsValidAssistTarget flag
	// gates (Object.cpp:2972/3127/2991/3134, bundled in spellTargetUnitBlocked;
	// the NON_ATTACKABLE/TRIGGER/NO_COMBAT bundle always rejects on the
	// attack path but only for negative spells on the assist path,
	// Object.cpp:2980/3131)
	// and the hostility/faction gates (Object.cpp "can't attack friendly
	// targets" / "can't assist non-friendly targets", plus the PARTY/RAID
	// membership terms, via explicitTargetFactionBlocked), failing the cast
	// with SPELL_FAILED_BAD_TARGETS (SharedDefines.h:992). The
	// SpellInfo::CheckTarget GM/invisibility gate (SpellInfo.cpp:1736-1743,
	// explicitTargetGMBlocked) fails the cast with
	// SPELL_FAILED_BM_OR_INVISGOD (SharedDefines.h:1141).
	// Self is exempt: IsValidAssistTarget returns true for self (Object.cpp:3092).
	explicitUnitGUID := uint64(0)
	if isSelfCastOnly(spell) {
		hitTargets = append(hitTargets, s.playerGUID)
	} else if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		explicitUnitGUID = target.UnitGUID
	} else if s.selection != 0 {
		explicitUnitGUID = s.selection
	} else if !isHarmfulSpell(spell) {
		hitTargets = append(hitTargets, s.playerGUID)
	}
	if explicitUnitGUID != 0 && explicitUnitGUID != s.playerGUID {
		if tgt, ok := s.getCombatTarget(ctx, explicitUnitGUID); ok {
			// SpellInfo.cpp:1799-1816: the ENEMY explicit mask runs the
			// Unit::IsValidAttackTarget flag gates (bundle always rejects,
			// Object.cpp:2980) and the ALLY/PARTY/RAID masks run the
			// WorldObject::IsValidAssistTarget gates (bundle only for
			// negative spells, Object.cpp:3131).
			explicitMask := spellExplicitUnitTargetMask(spell)
			assist := explicitMask&(targetFlagUnitAlly|targetFlagUnitParty|targetFlagUnitRaid) != 0
			if spellTargetUnitBlocked(spell, tgt.UnitFlags, tgt.FlagsExtra, assist) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBadTargets), true)
				s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "explicit target blocked")
				return
			}
			// SpellInfo.cpp:1799-1816: the ENEMY/ALLY/PARTY/RAID explicit
			// masks carry the hostility/faction gates.
			if s.explicitTargetFactionBlocked(explicitMask, explicitUnitGUID, tgt) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBadTargets), true)
				s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "explicit target faction mismatch")
				return
			}
			// SpellInfo.cpp:1736-1743: GM-invisible or GM-mode player targets
			// reject the cast with SPELL_FAILED_BM_OR_INVISGOD; self is
			// exempt (this block only runs when explicitUnitGUID != s.playerGUID).
			if s.explicitTargetGMBlocked(explicitUnitGUID) {
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedBmOrInvisGod), true)
				s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "explicit target GM/invisible")
				return
			}
		}
		hitTargets = append(hitTargets, explicitUnitGUID)
	}
	areaSpell := isAreaEnemySpell(spell)
	friendlyAreaSpell := isFriendlyAreaSpell(spell)
	friendlyNearbySpell := isFriendlyNearbySpell(spell)
	entryNearbySpell := isEntryNearbySpell(spell)
	goNearbyEntrySpell := isGONearbyEntrySpell(spell)
	entryAreaSpell := isEntryAreaSpell(spell)
	goAreaSpell := isGOAreaSpell(spell)
	goConeSpell := isGOConeSpell(spell)
	friendlyConeSpell := isFriendlyConeSpell(spell)
	friendlyLastTargetAreaSpell := isFriendlyLastTargetAreaSpell(spell)
	friendlyTargetAreaRaidClassSpell := isFriendlyTargetAreaRaidClassSpell(spell)
	// List-producing friendly/entry selections skip the single-target immune gate
	// and chain-jump expansion the same way area spells do (Spell.cpp:1227
	// area selection never calls SelectImplicitChainTargets).
	friendlyListSpell := friendlyAreaSpell || friendlyConeSpell || friendlyLastTargetAreaSpell || friendlyTargetAreaRaidClassSpell || entryAreaSpell || goAreaSpell || goConeSpell
	if areaSpell {
		hitTargets = s.spellAreaEnemyTargets(ctx, spell, target)
	} else if friendlyAreaSpell {
		// Spell::SelectImplicitAreaTargets (Spell.cpp:1227): friendly
		// PARTY/ALLY/RAID area targets (20/30/31/33/34/56) replace the
		// hit-target list the same way enemy area targets do.
		hitTargets = s.spellFriendlyAreaTargets(ctx, spell, target)
	} else if friendlyNearbySpell {
		// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036): the single
		// nearest PARTY/ALLY/RAID unit (3/4/58) becomes the target.
		if nearby, ok := s.spellFriendlyNearbyTarget(ctx, spell, target); ok {
			hitTargets = []uint64{nearby}
		} else {
			// Spell.cpp:1111: no target found ->
			// SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993).
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 11), true)
			s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "no nearby target")
			return
		}
	} else if entryNearbySpell {
		// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036):
		// TARGET_UNIT_NEARBY_ENTRY (38) — the single nearest entry-matched
		// unit becomes the target; no match fails the cast.
		if nearby, ok := s.spellEntryNearbyTarget(ctx, spell, spellID); ok {
			hitTargets = []uint64{nearby}
		} else {
			// Spell.cpp:1111: no target found ->
			// SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993).
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 11), true)
			s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "no entry target")
			return
		}
	} else if entryAreaSpell {
		// Spell::SelectImplicitAreaTargets (Spell.cpp:1227):
		// TARGET_UNIT_SRC_AREA_ENTRY (7) / TARGET_UNIT_DEST_AREA_ENTRY (8)
		// — every unit in the area matching the entry conditions becomes a
		// target; an empty area never fails the cast.
		hitTargets = s.spellEntryAreaTargets(ctx, spell, spellID, target)
	} else if goNearbyEntrySpell {
		// Spell::SelectImplicitNearbyTargets (Spell.cpp:1036):
		// TARGET_GAMEOBJECT_NEARBY_ENTRY (40) — the single nearest
		// entry-matched gameobject becomes the target; no match fails the
		// cast. The GO guid flows through hitTargets like C++'s AddGOTarget
		// list; Go has no gameobject-effect consumer downstream.
		if nearby, ok := s.spellEntryNearbyGOTarget(ctx, spell, spellID); ok {
			hitTargets = []uint64{nearby}
		} else {
			// Spell.cpp:1111: no target found ->
			// SPELL_FAILED_BAD_IMPLICIT_TARGETS (SharedDefines.h:993).
			_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, 11), true)
			s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "no nearby gameobject target")
			return
		}
	} else if friendlyConeSpell {
		// Spell::SelectImplicitConeTargets (Spell.cpp:1176): friendly
		// ALLY/ENTRY cone targets (59/60).
		hitTargets = s.spellFriendlyConeTargets(ctx, spell, target)
	} else if goAreaSpell {
		// Spell::SelectImplicitAreaTargets (Spell.cpp:1227):
		// TARGET_GAMEOBJECT_SRC_AREA (51) / TARGET_GAMEOBJECT_DEST_AREA
		// (52) — every gameobject in the area becomes a target; an empty
		// area never fails the cast. The GO guids flow through hitTargets
		// like C++'s AddGOTarget list; Go has no gameobject-effect
		// consumer downstream.
		hitTargets = s.spellGOAreaTargets(ctx, spell, spellID, target)
	} else if goConeSpell {
		// Spell::SelectImplicitConeTargets (Spell.cpp:1176):
		// TARGET_GAMEOBJECT_CONE (108) — every gameobject in the caster's
		// front cone becomes a target; an empty cone never fails the cast.
		hitTargets = s.spellGOConeTargets(ctx, spell, spellID)
	} else if friendlyLastTargetAreaSpell || friendlyTargetAreaRaidClassSpell {
		// Spell::SelectImplicitAreaTargets (Spell.cpp:1227) with LAST (37)
		// or TARGET (61) reference: area around the last/explicit target.
		hitTargets = s.spellFriendlyRefCenteredAreaTargets(ctx, spell, target, hitTargets)
	}
	s.spawnPersistentAreaAura(ctx, spell, target)
	targetGUID := uint64(0)
	if len(hitTargets) > 0 {
		targetGUID = hitTargets[0]
	}

	// Auto-repeat ranged spells (e.g. Auto Shot, Shoot Wand) (TC: CURRENT_AUTOREPEAT_SPELL)
	if (spell.AttributesEx1&spellAttr2AutoRepeatFlag != 0) || spellID == 75 || spellID == 5019 {
		s.autoRepeatSpell = spellID
		s.autoRepeatTarget = targetGUID
		if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
			s.executeRangedAttack(ctx, tgt, spellID)
		}
		return
	}

	// Spell hit check for offensive spells targeting another unit
	var missStatus []protocol.SpellMissStatus
	isReflected := false
	if !areaSpell && targetGUID != 0 && targetGUID != s.playerGUID && isHarmfulSpell(spell) {
		var targetSess *session
		if s.server != nil {
			targetSess = s.server.findSessionByGUID(targetGUID)
		}
		if targetSess != nil && targetSess.checkSpellReflection(spell) {
			isReflected = true
			missStatus = []protocol.SpellMissStatus{{
				TargetGUID:    targetGUID,
				Reason:        protocol.SpellMissReflect,
				ReflectStatus: 2,
			}}
			targetGUID = s.playerGUID
			hitTargets = []uint64{s.playerGUID}
		} else if targetSess != nil && targetSess.isImmuneToSpell(spell) {
			hitTargets = nil
			missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: protocol.SpellMissImmune}}
		} else {
			targetLevel := uint8(1)
			if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
				targetLevel = tgt.Level
			}
			isPlayerVictim := targetSess != nil
			bonusHit := 0.0
			if s.player != nil {
				bonusHit = s.getSpellHitPct()
			}
			missInfo := magicSpellHitResult(s.player.Level, targetLevel, isPlayerVictim, bonusHit)
			if missInfo != protocol.SpellMissNone {
				hitTargets = nil
				missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: missInfo}}
			} else if isBinarySpell(spell) {
				var resistances [7]uint32
				if targetSess != nil && targetSess.player != nil {
					resistances = targetSess.player.Resistances
				} else if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok {
					resistances = tgt.Resistances
				}
				pen := uint32(0)
				if s.player != nil {
					pen = s.player.SpellPenetration
				}
				chaosBolt := spell.SpellFamilyName == spellFamilyWarlock && spell.SpellIconID == 3178
				if checkBinarySpellResist(resistances, uint8(spell.SchoolMask), pen, s.player.Level, targetLevel, chaosBolt) {
					hitTargets = nil
					missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: protocol.SpellMissResist}}
				}
			}
		}
	} else if !areaSpell && !friendlyListSpell && targetGUID != 0 && targetGUID != s.playerGUID && !isHarmfulSpell(spell) {
		var targetSess *session
		if s.server != nil {
			targetSess = s.server.findSessionByGUID(targetGUID)
		}
		if targetSess != nil && targetSess.isImmuneToSpell(spell) {
			hitTargets = nil
			missStatus = []protocol.SpellMissStatus{{TargetGUID: targetGUID, Reason: protocol.SpellMissImmune}}
		}
	}

	// Spell::SelectImplicitChainTargets (Spell.cpp:1582): chain spells
	// (EffectChainTarget > 1) jump from the primary target to nearby units;
	// chainJumpIndex records each target's jump order (0 = primary) so the
	// per-jump EffectChainAmplitude falloff can be applied at effect time.
	chainJumpIndex := make(map[uint64]int)
	if !areaSpell && !friendlyListSpell && targetGUID != 0 {
		if jumps, isChainHeal := chainSpellJumps(spell); jumps > 0 {
			for i, extraGUID := range s.spellSearchChainTargets(ctx, spell, targetGUID, jumps, isChainHeal) {
				hitTargets = append(hitTargets, extraGUID)
				chainJumpIndex[extraGUID] = i + 1
			}
		}
	}

	// Spell::prepare (Spell.cpp:3040-3056) fills m_auraScaleMask for
	// player-cast, non-passive, SpellLevel-carrying, non-channeled,
	// non-triggered buff spells; the AddUnitTarget min-level arm
	// (Spell.cpp:2115-2150) and the SelectTargets removal leg
	// (Spell.cpp:807-830) then drop targets failing the targetLevel+10 >=
	// firstRank.SpellLevel check, failing the cast with
	// SPELL_FAILED_LOWLEVEL when none remain. finishSpellCast only serves
	// client-initiated casts — every Go triggered cast rides
	// TRIGGERED_FULL_MASK, which carries TRIGGERED_IGNORE_AURA_SCALING
	// (0x10) — so the triggered gate is structural here. The removal runs
	// before TakePower/TakeReagents below, matching _cast order
	// (SelectSpellTargets at Spell.cpp:3410 precedes TakePower at 3452).
	var auraScaleDownranks map[uint64]wotlk.Spell
	if s.server != nil && s.server.Data != nil {
		if pristine, found, _ := s.server.Data.Spell(spellID); found {
			if mask := spellAuraScaleMask(spell, pristine, true); mask != 0 {
				var failed bool
				hitTargets, auraScaleDownranks, failed = s.applyAuraScaling(ctx, spellID, spell, mask, hitTargets)
				if failed {
					_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedLowLevel), true) // SPELL_FAILED_LOWLEVEL = 48
					s.debug("spell cast failed at completion", "account", s.accountName, "spell", spellID, "reason", "aura scale min level")
					return
				}
				if len(hitTargets) > 0 {
					targetGUID = hitTargets[0]
				}
			}
		}
	}

	castTimeStamp := uint32(time.Now().UnixMilli())
	castFlags := spellCastFlagGo
	var remainingPower *uint32
	if pType < 7 {
		castFlags |= protocol.SpellCastFlagPowerLeftSelf
		power := s.player.Powers[pType]
		remainingPower = &power
	}
	if pType < 7 && cost > 0 {
		// Re-validation above guarantees sufficient power; C++ TakePower deducts.
		s.player.Powers[pType] -= cost
		powerPacket := protocol.NewBuffer(13)
		powerPacket.WritePackedGUID(s.playerGUID)
		powerPacket.WriteU8(uint8(pType))
		powerPacket.WriteU32(s.player.Powers[pType])
		_ = s.write(uint16(protocol.OpcodeSMSG_POWER_UPDATE), powerPacket.Bytes(), true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_POWER_UPDATE), powerPacket.Bytes(), s)
		}
		if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			col := fmt.Sprintf("power%d", pType+1)
			_, _ = s.server.CharactersStore.DB.ExecContext(ctx, fmt.Sprintf("UPDATE characters SET %s = ? WHERE guid = ?", col), s.player.Powers[pType], s.playerGUID)
		}
	}
	s.takeSpellReagents(ctx, spell)

	// Spell::TakePower (Spell.cpp:4838-4844) spends runes for POWER_RUNE
	// spells via TakeRunePower. didHit mirrors C++ (false only when the
	// primary target missed; chain jumps appended to hitTargets above
	// must not flip it).
	didHit := true
	for _, miss := range missStatus {
		if miss.TargetGUID == targetGUID {
			didHit = false
			break
		}
	}
	if spell.PowerType == 5 {
		s.takeRunePower(ctx, spell, didHit, time.Now().UnixMilli())
		s.sendRuneCooldownUpdate()
	}

	// Spell::_cast (Spell.cpp:3416-3420): SPELL_ATTR1_DISMISS_PET dismisses
	// the caster's active pet at cast time, before the cooldown packet and
	// SendSpellGo below — without this attribute, summoning spells fail when
	// the caster already has a pet (SharedDefines.h:449).
	if spell.AttributesEx&spellAttr1DismissPet != 0 {
		s.unsummonPet(ctx, petSaveNotInSlot)
	}

	// Spell::_cast (Spell.cpp:3462-3470) calls SendSpellCooldown() before
	// HandleLaunchPhase() and SendSpellGo(): the cooldown packet must reach
	// the client before SMSG_SPELL_GO.
	categoryID, categoryRecoveryTime := spell.Category, spell.CategoryRecoveryTime
	now := time.Now()
	categoryEnd := now.Unix()
	if categoryRecoveryTime > 0 {
		categoryEnd = now.Add(time.Duration(categoryRecoveryTime) * time.Millisecond).Unix()
	}
	applyCooldown := len(hitTargets) > 0 && !isFishingSpell(spellID)
	if applyCooldown && (spell.RecoveryTime > 0 || categoryRecoveryTime > 0) {
		cooldownEnd := categoryEnd
		if spell.RecoveryTime > 0 {
			cooldownEnd = now.Add(time.Duration(spell.RecoveryTime) * time.Millisecond).Unix()
		}
		s.player.Cooldowns = append(s.player.Cooldowns, spellCooldown{Spell: spellID, Category: categoryID, End: cooldownEnd, CategoryEnd: categoryEnd})
	}
	if applyCooldown && spell.RecoveryTime > 0 {
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_COOLDOWN), buildSpellCooldown(s.playerGUID, spellID, spell.RecoveryTime), true)
	}
	if applyCooldown && spellID == 8690 {
		cooldownEnd := now.Add(15 * time.Minute).Unix() // 15 min cooldown
		s.player.Cooldowns = append(s.player.Cooldowns, spellCooldown{Spell: spellID, Item: 6948, Category: categoryID, End: cooldownEnd, CategoryEnd: categoryEnd})
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_COOLDOWN), buildSpellCooldown(s.playerGUID, spellID, 900000), true)
	}

	// Spell::HandleLaunchPhase (Spell.cpp:7685-7726) runs at launch, between
	// SendSpellCooldown and SendSpellGo: the SPELL_EFFECT_HANDLE_LAUNCH /
	// SPELL_EFFECT_HANDLE_LAUNCH_TARGET effect modes, the
	// DoEffectOnLaunchTarget combat engage, and TakeAmmo for player
	// SPELL_ATTR0_REQ_AMMO spells. Only the ammo leg has a Go bridge:
	//   - LAUNCH / LAUNCH_TARGET modes have no Go equivalent; all effects
	//     resolve at hit time in applyEffects.
	//   - The launch-time SetInCombatWith engage is a timing delta: C++
	//     puts the caster in combat when the missile launches; Go engages
	//     (triggerCreatureAggro) when the effects hit.
	//   - The triggered-cast Volley-tick exemption (SPELLFAMILY_HUNTER +
	//     IsTargetingArea) is moot: finishSpellCast only serves
	//     player-initiated casts, so Volley's non-triggered initial cast
	//     consumes here at launch; the second ammo comes from the
	//     handle_immediate tail leg bridged after the finish-phase legs
	//     below (Spell.cpp:3620-3622).
	//   - TakeAmmo's wand / broken-ranged / thrown-weapon legs have no Go
	//     bridge: no ranged-slot or thrown-weapon model (wands never carry
	//     REQ_AMMO, so the wand leg is vacuous under this gate).
	if spell.Attributes&spellAttr0ReqAmmo != 0 && !s.hasConsumeNoAmmoAura(spell) {
		s.consumeRangedAmmo(ctx)
	}

	goPacket := protocol.BuildSpellGoWithPower(s.playerGUID, s.playerGUID, castID, spellID, castFlags, castTimeStamp, hitTargets, missStatus, target, remainingPower)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPacket, true)
	if s.server != nil {
		nearbyFlags := castFlags &^ protocol.SpellCastFlagPowerLeftSelf
		nearbyPacket := protocol.BuildSpellGo(s.playerGUID, s.playerGUID, castID, spellID, nearbyFlags, castTimeStamp, hitTargets, missStatus, target)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), nearbyPacket, s)
	}
	if isFishingSpell(spellID) {
		s.spawnFishingBobber(ctx, target)
		return
	}

	if len(hitTargets) == 0 {
		// Spell missed, do not trigger channel or effects
		return
	}

	// Spell::_handle_immediate_phase (Spell.cpp:3718): initial spell threat
	// (HandleThreatSpells, Spell.cpp:5096) lands before any effect handling.
	s.handleSpellInitialThreat(ctx, spell, hitTargets)

	// Reference Spell::handle_immediate: channeled spells begin their timed
	// channel lifecycle after the cast completes. The resolved destination is
	// recorded with the channel (C++ channeledSpell->m_targets dest), so
	// spells triggered during the channel can resolve TARGET_DEST_CHANNEL_TARGET.
	if isChanneledSpell(spell) {
		hasDest := target.Flags&protocol.SpellTargetFlagDestLocation != 0
		s.startChannel(castID, spellID, spell, targetGUID, hasDest, target.Destination.X, target.Destination.Y, target.Destination.Z)
	}
	s.updateAchievementCriteria(criteriaTypeCastSpell, spellID, 1)
	s.updateAchievementCriteria(criteriaTypeCastSpell2, spellID, 1)
	s.startTimedAchievement(timedTypeSpellCast, spellID)

	// Reference SpellEffects.cpp:3858-3874: Spell 7266 (Duel)
	if spellID == 7266 && targetGUID != 0 && targetGUID != s.playerGUID && s.server != nil {
		if partner := s.server.findSessionByGUID(targetGUID); partner != nil && partner.player != nil {
			s.duelPartner = targetGUID
			partner.duelPartner = s.playerGUID
			arbiterGUID := uint64(s.playerGUID) | (uint64(0xF110) << 48)
			s.player.DuelArbiter = arbiterGUID
			partner.player.DuelArbiter = arbiterGUID
			s.player.DuelTeam = 0
			partner.player.DuelTeam = 0
			s.sendPlayerUpdate()
			partner.sendPlayerUpdate()

			midX := s.player.X + (partner.player.X-s.player.X)/2
			midY := s.player.Y + (partner.player.Y-s.player.Y)/2
			midZ := s.player.Z
			s.duelArbiterX, s.duelArbiterY, s.duelArbiterZ = midX, midY, midZ
			partner.duelArbiterX, partner.duelArbiterY, partner.duelArbiterZ = midX, midY, midZ
			s.duelOutOfBounds = time.Time{}
			partner.duelOutOfBounds = time.Time{}

			reqBuf := protocol.NewBuffer(16)
			reqBuf.WriteU64(arbiterGUID)
			reqBuf.WriteU64(s.playerGUID)
			_ = s.write(uint16(protocol.OpcodeSMSG_DUEL_REQUESTED), reqBuf.Bytes(), true)
			_ = partner.write(uint16(protocol.OpcodeSMSG_DUEL_REQUESTED), reqBuf.Bytes(), true)
		}
	}

	// Apply spell effects
	// Spell::AddComboPointGain (Spell.h:515-522) per-cast bank: the
	// ADD_COMBO_POINTS effects bank (target, gain) pairs during the HIT
	// phase and _handle_finish_phase spends them after all targets are
	// processed (Spell.cpp:3738-3752).
	var comboGainTarget uint64
	var comboGain int8
	applyEffects := func(effCtx context.Context) {
		if len(missStatus) > 0 && !isReflected {
			return
		}
		// Eluna::SpellHit (CREATURE_EVENT_ON_HIT_BY_SPELL, event 14) fires
		// once per spell hit on a creature target: Spell.cpp:2626
		// (Spell::UnitTargetInfo::DoTargetSpellHit) calls CreatureAI::
		// SpellHit when _spellHitTarget->ToCreature(). A missed spell never
		// fires (_spellHitTarget is null when the spell misses in
		// DoSpellHitOnUnit), matching the miss guard above. Eluna argument
		// order is (event, creature, caster, spellid). ElunaCreatureAI::
		// SpellHit's truthy return vetoes only ScriptedAI::SpellHit, which
		// is empty (the CreatureAI.h:143 base is the only implementation),
		// so the return is discarded the way the enter-combat hook's is.
		// The caster is always the player session here: Go has no
		// creature-caster spell path, so the ON_SPELL_HIT_TARGET (event 15)
		// gate (caster TYPEID_UNIT with AI enabled) has no fire site.
		for _, effectTarget := range hitTargets {
			if uint16(effectTarget>>48) != 0xF130 {
				continue
			}
			if motion := s.findCreatureMotion(effectTarget); motion != nil {
				s.server.fireCreatureLuaEvent(effCtx, motion, scripting.CreatureEventOnHitBySpell, s.luaPlayer(), spellID)
			}
		}
		// Per-(cast, target) first-merge marker for the aura re-apply path
		// (Spell.cpp:2842): only the first aura effect per target runs the
		// ModStackAmount(+1) merge, later effects only refresh their amounts.
		castMerged := make(map[uint64]struct{})
		interruptHandled := false
		damageEffectSeen := false
		for effectIndex, eff := range spell.Effects {
			if eff.Effect == 0 {
				continue
			}
			switch eff.Effect {
			case 1: // SPELL_EFFECT_INSTAKILL
				damageEffectSeen = true
				for _, effectTarget := range hitTargets {
					if effectTarget != 0 {
						s.executeSpellInstantKill(effCtx, effectTarget, spellID)
					}
				}
			case 2, 17, 31, 58, 87: // Damage effects (School damage, Weapon damage, etc.)
				damageEffectSeen = true
				damage := uint32(eff.BasePoints + 1)
				for _, effectTarget := range hitTargets {
					if effectTarget != 0 && (effectTarget != s.playerGUID || isReflected) {
						s.executeSpellDamage(effCtx, effectTarget, spellID, chainScaledAmount(damage, eff, chainJumpIndex[effectTarget]), effectIndex)
					}
				}
			case 10, 136, 105: // Heal effects
				heal := uint32(eff.BasePoints + 1)
				for _, effectTarget := range hitTargets {
					s.executeSpellHeal(effCtx, effectTarget, spellID, chainScaledAmount(heal, eff, chainJumpIndex[effectTarget]), effectIndex)
				}
			case spellEffectEnergize:
				amount := eff.BasePoints + 1
				for _, effectTarget := range hitTargets {
					s.applySpellEnergize(effCtx, effectTarget, eff.MiscValue, amount)
				}
			case spellEffectPowerBurn:
				amount := eff.BasePoints + 1
				for _, effectTarget := range hitTargets {
					// SpellEffects.cpp:1383: the drained power is dealt as
					// damage scaled by the effect value multiplier.
					if burned := s.applySpellPowerBurn(effCtx, effectTarget, eff.MiscValue, amount, spellID); burned > 0 {
						s.executeSpellDamage(effCtx, effectTarget, spellID, effectValueMultiplied(burned, eff.Amplitude), effectIndex)
					}
				}
			case spellEffectParry:
				if s.player != nil && !s.player.CanParry {
					s.player.CanParry = true
					s.updatePlayerParryPercentage(s.player, s.player.Level)
					s.sendPlayerUpdate()
				}
			case spellEffectTriggerSpell:
				for _, effectTarget := range hitTargets {
					if eff.TriggerSpell != 0 && eff.TriggerSpell != spellID {
						s.castSpellDirect(effCtx, eff.TriggerSpell, effectTarget)
					}
				}
			case spellEffectAddExtraAttacks: // 19: SPELL_EFFECT_ADD_EXTRA_ATTACKS
				// Spell::EffectAddExtraAttacks (SpellEffects.cpp:4301-4314)
				// banks the effect's damage as pending extra swings on the
				// effect's unit target when none are pending. The known
				// extra-attack spells resolve that target to the caster
				// (TARGET_UNIT_CASTER), and Go's player-centric combat models
				// the counter on the caster session only.
				amount := uint32(eff.BasePoints + 1)
				for _, effectTarget := range hitTargets {
					if effectTarget == s.playerGUID && s.grantExtraAttacks(amount) {
						s.sendExtraAttacksLog(spellID, effectIndex, amount)
					}
				}
			case spellEffectAddComboPoints: // 80: SPELL_EFFECT_ADD_COMBO_POINTS
				// Spell::EffectAddComboPoints (SpellEffects.cpp:3781-3789)
				// banks the effect's damage as a per-cast combo-point gain
				// (Spell::AddComboPointGain, Spell.h:515-522): the
				// effectHandleMode gate is structural — this dispatch is the
				// HIT_TARGET phase — and damage <= 0 gains nothing. Each
				// hit unit target is a separate gain: a new target restarts
				// the bank, the same target accumulates; the spend happens
				// in _handle_finish_phase after all targets are processed.
				gain := int8(eff.BasePoints + 1)
				if gain > 0 {
					for _, effectTarget := range hitTargets {
						if effectTarget == 0 {
							continue
						}
						if effectTarget != comboGainTarget {
							comboGainTarget = effectTarget
							comboGain = gain
						} else {
							comboGain += gain
						}
					}
				}
			case 3:
				s.addOwnerPetAuraSource(effCtx, spellID, uint8(effectIndex))
			case spellEffectThreat:
				amount := eff.BasePoints + 1
				for _, effectTarget := range hitTargets {
					s.applySpellThreat(effCtx, effectTarget, amount)
				}
			case spellEffectHealMaxHealth:
				for _, effectTarget := range hitTargets {
					s.executeSpellMaxHealthHeal(effCtx, effectTarget, spellID)
				}
			case 6, 27, 35: // Apply Aura
				durationMs, periodMs, amount := s.auraEffectParams(spell, eff)
				schoolMask := spell.SchoolMask
				if schoolMask == 0 {
					schoolMask = 1
				}

				for _, effectTarget := range hitTargets {
					auraTarget := s.playerGUID
					if isHarmfulSpell(spell) && isAreaEnemySpell(spell) && effectTarget != 0 && effectTarget != s.playerGUID {
						auraTarget = effectTarget
					} else if eff.ImplicitTargetA == 1 || isSelfCastOnly(spell) || isReflected {
						auraTarget = s.playerGUID
					} else if eff.ImplicitTargetA == 6 || isHarmfulAura(eff.Aura) {
						if effectTarget != 0 && effectTarget != s.playerGUID {
							auraTarget = effectTarget
						}
					} else if eff.ImplicitTargetA == 21 {
						if effectTarget != 0 {
							auraTarget = effectTarget
						}
					} else if effectTarget != 0 && effectTarget != s.playerGUID && isHarmfulSpell(spell) {
						auraTarget = effectTarget
					}
					// Spell::PreprocessSpellHit scaleAura leg (Spell.cpp:2780-2795):
					// a target that passed the aura-scale min-level check gets
					// the aura from the rank-appropriate SpellInfo with that
					// rank's basepoints (DoTargetSpellHit creates the aura from
					// hitInfo.AuraSpellInfo, Spell.cpp:2858-2861).
					effSpell, effEff := spell, eff
					tgtDurationMs, tgtPeriodMs, tgtAmount := durationMs, periodMs, amount
					if ds, ok := auraScaleDownranks[auraTarget]; ok {
						effSpell = ds
						if effectIndex < len(ds.Effects) {
							effEff = ds.Effects[effectIndex]
						}
						tgtDurationMs, tgtPeriodMs, tgtAmount = s.auraEffectParams(effSpell, effEff)
					}
					s.applyAuraToTarget(effCtx, auraTarget, effSpell, effEff, tgtDurationMs, tgtPeriodMs, tgtAmount, schoolMask, castMerged, false, s.playerGUID)
				}
			case spellEffectResurrectNew: // SPELL_EFFECT_RESURRECT_NEW: self resurrect chain
				s.applySelfResurrectEffect(spell)
			case 5: // SPELL_EFFECT_TELEPORT_UNITS (e.g. Hearthstone 8690, Astral Recall 556)
				if (spellID == 8690 || spellID == 556) && s.player != nil {
					hbMap, hbX, hbY, hbZ := s.player.HomebindMap, s.player.HomebindX, s.player.HomebindY, s.player.HomebindZ
					if hbX == 0 && hbY == 0 && hbZ == 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
						_ = s.server.WorldStore.DB.QueryRowContext(effCtx, "SELECT map, position_x, position_y, position_z FROM playercreateinfo WHERE race = ? AND class = ? LIMIT 1", s.player.Race, s.player.Class).Scan(&hbMap, &hbX, &hbY, &hbZ)
					}
					s.teleportTo(hbMap, hbX, hbY, hbZ, 0)
				}
			case 162: // SPELL_EFFECT_TALENT_SPEC_SELECT
				targetSpec := uint8(0)
				if eff.BasePoints+1 >= 2 {
					targetSpec = 1
				}
				s.activateSpec(effCtx, targetSpec)
			case 74: // SPELL_EFFECT_APPLY_GLYPH
				glyphPropID := uint16(eff.MiscValue)
				s.applyGlyph(effCtx, s.targetGlyphSlot, glyphPropID)
			case 56: // SPELL_EFFECT_SUMMON_PET
				s.handleSummonPet(effCtx, spellID, uint32(eff.MiscValue))
			case 28: // SPELL_EFFECT_SUMMON
				s.handleSummonPet(effCtx, spellID, uint32(eff.MiscValue))
			case 101: // SPELL_EFFECT_FEED_PET
				s.handleFeedPet(effCtx, spellID, target.ItemGUID, eff.TriggerSpell, uint8(effectIndex))
			case 102: // SPELL_EFFECT_DISMISS_PET
				s.handleDismissPet(effCtx)
			case 109: // SPELL_EFFECT_RESURRECT_PET
				s.handleResurrectPet(effCtx, spellID)
			case 55: // SPELL_EFFECT_TAMECREATURE
				s.handleTameCreature(effCtx, spellID, targetGUID)
			case 135: // SPELL_EFFECT_CALL_PET
				s.handleSummonPet(effCtx, spellID, 0)
			case 38: // SPELL_EFFECT_DISPEL
				s.handleEffectDispel(effCtx, targetGUID, spell, eff)
			case 108: // SPELL_EFFECT_DISPEL_MECHANIC
				s.handleEffectDispelMechanic(effCtx, targetGUID, spell, eff)
			case 114: // SPELL_EFFECT_ATTACK_ME (EffectTaunt)
				s.handleEffectTaunt(effCtx, targetGUID, spellID)
			case 126: // SPELL_EFFECT_STEAL_BENEFICIAL_BUFF
				s.handleEffectSpellsteal(effCtx, targetGUID, spell, eff)
			case spellEffectInterruptCast: // 68: SPELL_EFFECT_INTERRUPT_CAST
				s.handleEffectInterruptCast(effCtx, targetGUID, spell, eff)
				interruptHandled = true
			case spellEffectCreateItem: // 24: SPELL_EFFECT_CREATE_ITEM
				s.handleEffectCreateItem(effCtx, targetGUID, spell, eff)
			case spellEffectCreateItem2: // 70: SPELL_EFFECT_CREATE_ITEM_2
				s.handleEffectCreateItem(effCtx, targetGUID, spell, eff)
			case spellEffectLearnSpell: // 36: SPELL_EFFECT_LEARN_SPELL
				if eff.TriggerSpell != 0 {
					s.learnSpell(effCtx, eff.TriggerSpell)
				}
			case spellEffectResurrect: // 18: SPELL_EFFECT_RESURRECT
				s.handleEffectResurrect(effCtx, targetGUID, spell, eff)
			case spellEffectReputation: // 103: SPELL_EFFECT_REPUTATION
				if eff.MiscValue != 0 {
					s.giveReputation(effCtx, uint32(eff.MiscValue), eff.BasePoints+1)
				}
			case spellEffectQuestComplete: // 16: SPELL_EFFECT_QUEST_COMPLETE
				if eff.MiscValue != 0 {
					s.completeQuest(effCtx, uint32(eff.MiscValue))
				}
			case spellEffectHealthLeech: // 9: SPELL_EFFECT_HEALTH_LEECH
				s.handleEffectHealthLeech(effCtx, spellID, hitTargets, effectIndex, eff)
			case spellEffectPowerDrain: // 8: SPELL_EFFECT_POWER_DRAIN
				amount := eff.BasePoints + 1
				// SpellEffects.cpp:1277-1282: the drain amount is direct
				// damage for the caster's SpellDamageBonusDone before the
				// drain, so the caster's spell power scales it. Go models
				// the spellpower term of SpellDamageBonusDone (see
				// executeSpellDamage); the damage-taken side has no Go
				// infra.
				if s.player != nil && s.player.SpellPower > 0 {
					amount += int32(math.Round(float64(s.player.SpellPower) * s.spellBonusMultiplier(spellID, effectIndex, false)))
				}
				for _, effectTarget := range hitTargets {
					// SpellEffects.cpp:1301: the caster regains the drained
					// power scaled by the effect value multiplier, never
					// from a self drain.
					if drained := s.applySpellPowerBurn(effCtx, effectTarget, eff.MiscValue, amount, spellID); drained > 0 {
						if effectTarget != s.playerGUID {
							s.applySpellEnergize(effCtx, s.playerGUID, eff.MiscValue, int32(effectValueMultiplied(drained, eff.Amplitude)))
						}
					}
				}
			default:
				s.debug("unhandled spell effect", "spell", spellID, "effect", eff.Effect, "index", effectIndex)
			}
		}
		// Spell::DoTriggersOnSpellHit (Spell.cpp:2913): the per-hit consumer
		// of the PrepareTriggersExecutedOnHit snapshot fires per unit hit
		// target after damage/healing is dealt (Spell.cpp:2650-2652), so it
		// runs here once the effects loop has applied everything to every
		// target. effMask is the mask of the spell's non-zero effects: Go
		// applies every non-zero effect to every hit target. The snapshot
		// and the linked-hit leg both fire from the same call even when the
		// snapshot is empty, matching C++.
		// The SpellScript OnHit/AfterHit legs around the same point
		// (Spell.cpp:2377 CallScriptOnHitHandlers; Spell.cpp:2659/2682-2707
		// CallScriptOnHitHandlers/CallScriptAfterHitHandlers in
		// DoTargetSpellHit) are document-only: there is no SpellScript
		// bridge (matching the no-SpellScript-bridge legs elsewhere), so
		// these handlers have nothing to invoke.
		if completedCast != nil {
			var hitEffMask uint8
			for effectIndex, eff := range spell.Effects {
				if eff.Effect != 0 {
					hitEffMask |= 1 << uint(effectIndex)
				}
			}
			for _, effectTarget := range hitTargets {
				s.fireHitTriggerSpells(effCtx, spell, spellID, hitEffMask, completedCast.HitTriggers, effectTarget)
			}
		}
		if s.server != nil && isHarmfulSpell(spell) && !damageEffectSeen {
			for _, effectTarget := range hitTargets {
				if effectTarget != 0 && effectTarget != s.playerGUID {
					s.server.triggerCreatureAggro(effCtx, effectTarget, s.playerGUID)
				}
			}
		}
		if isTauntSpell(spellID) {
			s.handleEffectTaunt(effCtx, targetGUID, spellID)
		}
		if isTotemSpell(spellID) {
			s.summonTotem(effCtx, spellID)
		} else if spellID == 36936 { // Totemic Recall
			s.destroyAllTotems()
		}
		if !interruptHandled && isInterruptSpell(spellID) {
			s.handleEffectInterruptCast(effCtx, targetGUID, spell, wotlk.SpellEffect{})
		}
		if spellID == 2641 { // Dismiss Pet
			s.handleDismissPet(effCtx)
		} else if spellID == 883 { // Call Pet
			s.handleSummonPet(effCtx, spellID, 0)
		} else if spellID == 31687 { // Summon Water Elemental
			s.handleSummonPet(effCtx, spellID, 510)
		} else if spellID == 46584 { // Raise Dead
			s.handleSummonPet(effCtx, spellID, 26125)
		} else if spellID == 63645 {
			s.activateSpec(effCtx, 0)
		} else if spellID == 63644 {
			s.activateSpec(effCtx, 1)
		} else if (spellID == 63624 || spellID == 63680) && s.player != nil && s.player.TalentGroupsCount < 2 {
			s.player.TalentGroupsCount = 2
			if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
				_, _ = s.server.CharactersStore.DB.ExecContext(effCtx, "UPDATE characters SET talentGroupsCount = 2 WHERE guid = ?", s.playerGUID)
			}
			_ = s.sendTalentsInfo(false)
		}
	}

	// Spell::_cast (Spell.cpp:3473-3494): the delayed-vs-immediate branch.
	// (Speed > 0 && !channeled) || SPELL_ATTR4_UNK4 ("Treat as delayed
	// spell", SharedDefines.h:564) takes the delayed path: TakeCastItem(),
	// m_spellState = SPELL_STATE_DELAYED + SetDelayStart(0), and the
	// UNIT_STATE_CASTING clear (unless another spell is being cast).
	// Go has no bridge for those legs: TakeCastItem only decrements
	// SpellCharges on the held cast item and Go has no item spell-charge
	// model (consumables are decremented at cast start in handleUseItem);
	// there is no SPELL_STATE machine or unit-state model. The observable
	// part — deferring effect execution to missile arrival — is this
	// branch (the pre-existing travel-delay code; Spell.cpp:2156-2169).
	// A UNK4-only spell (Speed == 0) still takes the delayed path in C++,
	// but SetDelayStart(0) with no travel speed means the delay timer
	// fires on the next update tick — behaviorally identical to the
	// immediate path, which is where it falls through here.
	// CallScriptAfterCastHandlers is a no-op (no SpellScript bridge).
	isDelayedBranch := (spell.Speed > 0 && !isChanneledSpell(spell)) || spell.AttributesEx4&spellAttr4TreatAsDelayed != 0
	if isDelayedBranch && targetGUID != 0 && targetGUID != s.playerGUID && spell.Speed > 0 {
		dist := float32(20.0) // default 20 yards if positions unknown
		if target, ok := s.getCombatTarget(ctx, targetGUID); ok {
			dx := target.X - s.player.X
			dy := target.Y - s.player.Y
			dz := target.Z - s.player.Z
			computedDist := float32(math.Sqrt(float64(dx*dx + dy*dy + dz*dz)))
			if computedDist > 5.0 {
				dist = computedDist
			} else {
				dist = 5.0
			}
		}
		timeDelayMs := int(math.Floor(float64(dist) / float64(spell.Speed) * 1000.0))
		if timeDelayMs > 0 {
			if timeDelayMs > 4000 {
				timeDelayMs = 4000
			}
			time.AfterFunc(time.Duration(timeDelayMs)*time.Millisecond, func() {
				// Spell::handle_delayed (Spell.cpp:3640): the delayed phase
				// holds the taking window around target processing; the
				// deferred end mirrors the tail (Spell.cpp:3697).
				s.beginSpellModTaking()
				defer s.endSpellModTaking()
				applyEffects(context.Background())
				s.consumeExtraAttacks(context.Background(), spellExtraAttackVictim(target, explicitUnitGUID))
				s.procSpellFinishAuraTriggers(context.Background(), spell)
				s.stopAttackOnSpellFinish(spell)
			})
			// Spell::handle_delayed (Spell.cpp:3629) no-bridge legs, noted:
			//   - UpdatePointers() fail -> finish(false): targets are
			//     resolved at cast start; the arrival closure does not
			//     re-resolve or fail the cast when a target vanished
			//     mid-flight (no pointer model).
			//   - SetSpellModTakingSpell(true/false) around the delayed
			//     ticks (Spell.cpp:3640/3697): bridged — the arrival closure
			//     opens its own taking context (spellmod.go); it starts
			//     fresh rather than inheriting the _cast registrations
			//     because Go has no single cast object spanning the phases
			//     (C++ keeps Spell::m_appliedMods for the Spell's whole
			//     lifetime). Noted delta.
			//   - Per-target TimeDelay waves: C++ staggers multi-target
			//     landings by distance (single_missile when HasDst(),
			//     else per-target t_offset waves with next_time
			//     rescheduling); Go fires one timer on the explicit
			//     target's travel time and lands all targets together.
			//   - m_UniqueGOTargetInfo recheck: Go has no gameobject/corpse/
			//     item target containers (unit targets only).
			//   - handle_delayed's DoProcessTargetContainer(delayedTargets)
			//     per-tick processing (PreprocessTarget, DoTargetSpellHit,
			//     DoDamageAndTriggers): Go's applyEffects is the HIT-mode
			//     phase for all resolved targets; the per-hit consumer
			//     DoTriggersOnSpellHit (Spell.cpp:2913) is bridged as
			//     fireHitTriggerSpells at the end of the effects loop — the
			//     ADD_TARGET_TRIGGER snapshot casts (roll_chance_i gate,
			//     triggered-spell duration propagation) and the spell_linked
			//     (id + SPELL_LINK_HIT) remove/apply rows.
			// The m_immediateHandled leg is parity: applyEffects (the
			// HIT-mode phase) runs at missile arrival in the deferred
			// closure, matching _handle_immediate_phase on the first
			// handle_delayed tick; handleSpellInitialThreat ran at cast
			// start like HandleThreatSpells at the _cast/_handle_immediate
			// head.
			// Spell::_cast (Spell.cpp:3502-3511): the spell_linked_spell tail
			// runs at _cast end on both branches — linked triggers fire at
			// cast completion, not at missile arrival.
			// Spell::_cast (Spell.cpp:3517-3523): the CHEAT_COOLDOWN leg
			// clears the just-cast spell's own cooldown here, between the
			// spell_linked tail and the proc leg below.
			// Spell::_cast (Spell.cpp:3525-3545): the "Handle procs on cast"
			// leg fires PROC_SPELL_PHASE_CAST right after the spell_linked
			// tail, on both the delayed and immediate branches.
			s.fireSpellLinkedTriggers(ctx, spellID, targetGUID)
			s.resetCastCooldownCheat(spellID)
			s.procSpellCastPhaseAuraTriggers(ctx, spell)
			return
		}
	}

	applyEffects(ctx)
	// Spell::handle_immediate (Spell.cpp:3568) no-bridge legs, noted:
	//   - PrepareTargetProcessing()/FinishTargetProcessing(): Go has no
	//     target container model; hitTargets were resolved at cast start.
	//   - DoProcessTargetContainer(m_UniqueGOTargetInfo/m_UniqueCorpseTargetInfo/
	//     m_UniqueItemInfo): Go processes unit targets only.
	// Spell::_handle_finish_phase (Spell.cpp:3738-3752): combo-point legs.
	// A spell requiring combo points (m_needComboPoints, Spell.cpp:532 =
	// SpellInfo::NeedsComboPoints) spends the banked points — before the
	// gain lands, matching the C++ ClearComboPoints-then-AddComboPoints
	// order. Item casts never take combo points (Spell.cpp:3097, the
	// m_CastItem arm of the prepare-time reset); the triggered-cast arm
	// of that reset is structural here — finishSpellCast serves only the
	// client cast path, triggered casts go through
	// castSpellDirectWithOverrides. The dodge/miss arm (Spell.cpp:2606:
	// no take on dodge and miss) is structural too: applyEffects returns
	// early for missed spells. The RETAIN_COMBO_POINTS aura removal
	// (Spell.cpp:3747-3750) has no Go bridge — the Go aura model tracks
	// no such aura type. The ABILITY_IGNORE_AURASTATE override
	// (Spell.cpp:5286) has no bridge for the same reason.
	if spellNeedsComboPoints(spell) && castItemGUID == 0 {
		s.clearSessionComboPoints()
	}
	if comboGainTarget != 0 && comboGain > 0 {
		s.addSessionComboPoints(comboGainTarget, comboGain)
	}
	// Spell::_handle_finish_phase (Spell.cpp:3738) no-bridge legs, noted:
	//   - ProcSkillsAndAuras(..., PROC_SPELL_PHASE_FINISH, m_hitMask):
	//     bridged as procSpellFinishAuraTriggers after the extra-attacks
	//     leg, matching _handle_finish_phase order (Spell.cpp:3753-3777).
	//     The DoTriggersOnSpellHit consumer is bridged separately as
	//     fireHitTriggerSpells at the end of the effects loop.
	// Spell::_handle_finish_phase (Spell.cpp:3753-3761): a finished cast
	// whose spell carries SPELL_EFFECT_ADD_EXTRA_ATTACKS spends the
	// caster's pending extra attacks as extra base-attack swings against
	// the cast's original unit target.
	s.consumeExtraAttacks(ctx, spellExtraAttackVictim(target, explicitUnitGUID))
	s.procSpellFinishAuraTriggers(ctx, spell)

	// Spell::handle_immediate tail (Spell.cpp:3616-3625):
	//   - TakeCastItem: no Go bridge — Go has no item spell-charge model;
	//     consumables are decremented at cast start in handleUseItem and
	//     the spell-charge decrement / expendable-destroy is unmodeled
	//     (standing gap, noted at the delayed-branch call above).
	//   - Volley ammo: IsRangedWeaponSpell() && IsChanneled() -> TakeAmmo().
	//     This is a second ammo on top of the HandleLaunchPhase REQ_AMMO
	//     consumption bridged at SendSpellGo above (C++ consumes once at
	//     launch and once here for the initial Volley cast).
	//   - finish(true) runs only when m_spellState != SPELL_STATE_CASTING:
	//     channeled spells skip it (Go's startChannel path) and the
	//     non-channeled completion below is the finish(true) Go model.
	// Delta: C++ also fires the tail TakeAmmo for triggered channeled
	// ranged casts (the per-tick Volley casts have no triggered exemption
	// on this leg); Go has no per-tick triggered casts (channelTick
	// applies tick damage directly), so ticks consume no ammo.
	if isChanneledSpell(spell) && isRangedWeaponSpell(spell) {
		s.consumeRangedAmmo(ctx)
	}
	s.stopAttackOnSpellFinish(spell)

	// Spell::finish(true) parity (Spell.cpp:3886-3985): two legs have no Go
	// bridge and are intentionally absent here.
	//   - IsAutoActionResetSpell -> resetAttackTimer(BASE/OFF/RANGED): the
	//     Go tree has no attack-timer model at all (melee swing timing is not
	//     simulated), so there is nothing to reset.
	//   - UpdatePotionCooldown (Player.cpp:22215): needs the last-used potion
	//     item id (m_lastPotionId, set in Spell::SendSpellCooldown) and a
	//     potion-cooldown event model; neither exists in Go.
	// The remaining finish legs have no Go bridge: UpdateInterruptMask
	// (IsChanneled) and the UNIT_STATE_CASTING clear have no model (Go
	// tracks cast/channel state in castMu, not unit states or interrupt
	// masks); the possessed-puppet unsummon and creature ReleaseSpellFocus
	// need charm/focus models Go does not have, and the statue unsummon
	// needs creature casters, which Go never creates (every cast is a
	// player session). SPELL_ATTR0_STOP_ATTACK_TARGET is covered by
	// stopAttackOnSpellFinish.

	// Spell::_cast (Spell.cpp:3502-3511): the spell_linked_spell tail runs
	// after handle_immediate for immediate spells — positive ids are cast
	// triggered on the unit target (or the caster when there is none),
	// negative ids remove the caster's auras of -id.
	// Spell::_cast (Spell.cpp:3517-3523): the CHEAT_COOLDOWN leg clears the
	// just-cast spell's own cooldown here, between the spell_linked tail
	// and the proc leg below.
	// Spell::_cast (Spell.cpp:3525-3545): the "Handle procs on cast" leg
	// fires PROC_SPELL_PHASE_CAST right after the spell_linked_spell tail,
	// on both the delayed and immediate branches. The C++ m_originalCaster
	// early-return gate is vacuous: finishSpellCast always runs on the
	// casting player session, and Go has no creature casters.
	s.fireSpellLinkedTriggers(ctx, spellID, targetGUID)
	s.resetCastCooldownCheat(spellID)
	s.procSpellCastPhaseAuraTriggers(ctx, spell)
}

// fireSpellLinkedTriggers applies the spell_linked_spell tail of
// Spell::_cast (Spell.cpp:3502-3511): for the plain spell id,
// sSpellMgr->GetSpellLinked returns the type-0 rows (SpellMgr::LoadSpellLinked
// shifts nonzero types into trigger ± SPELL_LINKED_MAX_SPELLS keys,
// SpellMgr.cpp:2167-2171 — those keys are consumed by the aura/hit hook
// paths, SpellAuras.cpp:1337 and Spell.cpp:2950, not by _cast). Negative ids
// remove the caster's auras of -id (Unit::RemoveAurasDueToSpell);
// positive ids are cast triggered on the unit target, or the caster when
// the cast has no unit target (m_targets.GetUnitTarget() ? ... : m_caster).
// The table is read on demand per the Go tree's no-in-memory-store
// convention; `reload spell_linked_spell` probes the same table.
func (s *session) fireSpellLinkedTriggers(ctx context.Context, spellID uint32, targetGUID uint64) {
	if s == nil || s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return
	}
	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT spell_effect FROM spell_linked_spell WHERE spell_trigger = ? AND type = 0", int32(spellID))
	if err != nil {
		if !missingTable(err) {
			s.debug("spell_linked_spell query failed", "spell", spellID, "err", err)
		}
		return
	}
	var effects []int32
	for rows.Next() {
		var effect int32
		if err := rows.Scan(&effect); err == nil {
			effects = append(effects, effect)
		}
	}
	rows.Close()
	for _, id := range effects {
		if id < 0 {
			// Unit::RemoveAurasDueToSpell(-id) on the caster: Go has no
			// caster aura-removal machine (documented as the missing
			// bridge in boss_ai.go), so the negative leg is noted here,
			// not fired.
			continue
		}
		if id == 0 {
			continue
		}
		tgt := targetGUID
		if tgt == 0 {
			tgt = s.playerGUID
		}
		s.castSpellDirect(ctx, uint32(id), tgt)
	}
}

// stopAttackOnSpellFinish stops the caster's auto-attack for spells carrying
// SPELL_ATTR0_STOP_ATTACK_TARGET.
// C++ authority: Spell::finish (Spell.cpp:3978-3983) calls AttackStop().
func (s *session) stopAttackOnSpellFinish(spell wotlk.Spell) {
	if spell.Attributes&spellAttr0StopAttackTarget == 0 {
		return
	}
	victim := s.attackTarget
	s.attackTarget = 0
	if s.autoRepeatSpell != 0 {
		s.autoRepeatSpell = 0
		s.autoRepeatTarget = 0
		buf := protocol.NewBuffer(9)
		buf.WritePackedGUID(s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
	}
	_ = s.sendAttackStop(victim, false)
	s.debug("attack stopped by spell", "account", s.accountName, "spell", spell.ID)
}

// spellExtraAttackVictim resolves the original unit target of a finished
// cast for the _handle_finish_phase extra-attacks arm (Spell.cpp:3755-3760 —
// m_targets.GetOrigUnitTargetGUID()). The parsed explicitUnitGUID already
// carries the packet-or-selection fallback for non-self casts; the
// self-cast branch skips that parse, so fall back to the packet's unit
// target there.
func spellExtraAttackVictim(target protocol.SpellTargetData, explicitUnitGUID uint64) uint64 {
	if explicitUnitGUID != 0 {
		return explicitUnitGUID
	}
	if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 {
		return target.UnitGUID
	}
	return 0
}

// grantExtraAttacks mirrors the counter arm of Spell::EffectAddExtraAttacks
// (SpellEffects.cpp:4301-4314): a live target with no pending extra attacks
// banks the effect's damage as extra base-attack swings. It reports whether
// the counter was set — C++ skips the log execute when attacks are already
// pending.
func (s *session) grantExtraAttacks(count uint32) bool {
	if s == nil || s.player == nil || s.isDeadOrGhost() || s.extraAttacks != 0 || count == 0 {
		return false
	}
	s.extraAttacks = count
	return true
}

// sendExtraAttacksLog mirrors Spell::ExecuteLogEffectExtraAttacks
// (Spell.cpp:4566-4571) as flushed by SendLogExecute (Spell.cpp:4523-4552):
// SMSG_SPELLLOGEXECUTE carrying the effect id, the target, and the banked
// attack count.
func (s *session) sendExtraAttacksLog(spellID uint32, effectIndex int, count uint32) {
	if s == nil || s.player == nil {
		return
	}
	log := protocol.NewBuffer(32)
	log.WritePackedGUID(s.playerGUID)
	log.WriteU32(spellID)
	log.WriteU32(1)
	log.WriteU32(spellEffectAddExtraAttacks)
	log.WritePackedGUID(s.playerGUID)
	log.WriteU32(count)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELLLOGEXECUTE), log.Bytes(), s)
	}
	_ = effectIndex
}

// consumeExtraAttacks mirrors Spell::_handle_finish_phase plus
// Unit::HandleProcExtraAttackFor (Spell.cpp:3753-3761, Unit.cpp:2180-2187):
// pending extra attacks become extra base-attack swings against the cast's
// original unit target, one decrement per swing (so the CheckEffectProc
// extra-attacks arm still sees the pending counter during those swings,
// blocking recursive extra-attack procs exactly like C++). A dead or
// unresolvable target burns the counter with no swings, matching the C++
// null-victim arm. Extra swings never reset the regular swing timer — the
// C++ extra=true arm skips the CURRENT_MELEE_SPELL cast and touches no
// timer — so lastSwing is preserved across them.
func (s *session) consumeExtraAttacks(ctx context.Context, targetGUID uint64) {
	if s == nil || s.player == nil || s.extraAttacks == 0 {
		return
	}
	defer func() { s.extraAttacks = 0 }()
	if targetGUID == 0 {
		return
	}
	for s.extraAttacks > 0 {
		target, ok := s.getCombatTarget(ctx, targetGUID)
		if !ok || target.Health == 0 {
			break
		}
		savedSwing := s.lastSwing
		s.executeMeleeSwing(ctx, target, protocol.BaseAttack)
		s.lastSwing = savedSwing
		s.extraAttacks--
	}
}

func (s *session) spawnPersistentAreaAura(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) {
	if s == nil || s.server == nil || s.player == nil {
		return
	}
	var persistent wotlk.SpellEffect
	found := false
	for _, effect := range spell.Effects {
		if effect.Effect == 27 {
			persistent, found = effect, true
			break
		}
	}
	if !found || spell.DurationIndex == 0 || s.server.Data == nil {
		return
	}
	durationMs, durationFound, err := s.server.Data.SpellDuration(spell.DurationIndex, uint32(s.player.Level))
	if err != nil || !durationFound || durationMs <= 0 {
		return
	}
	radius, radiusFound, err := s.server.Data.SpellRadius(persistent.RadiusIndex, uint32(s.player.Level))
	if err != nil || !radiusFound || radius <= 0 {
		return
	}
	x, y, z := s.player.X, s.player.Y, s.player.Z
	if target.Flags&protocol.SpellTargetFlagDestLocation != 0 {
		x, y, z = target.Destination.X, target.Destination.Y, target.Destination.Z
	} else if target.Flags&protocol.SpellTargetFlagUnitWireMask != 0 && target.UnitGUID != 0 {
		if destination, ok := s.getCombatTarget(ctx, target.UnitGUID); ok {
			x, y, z = destination.X, destination.Y, destination.Z
		}
	}
	lowGUID := s.server.nextDynamicSpellLowGUID()
	auraEffect := persistent
	for _, effect := range spell.Effects {
		if effect.Effect == 6 && effect.Aura != 0 {
			auraEffect = effect
			break
		}
	}
	periodMs := auraEffect.AuraPeriod
	if periodMs == 0 && (auraEffect.Aura == 3 || auraEffect.Aura == 8 || auraEffect.Aura == 23 || auraEffect.Aura == 24 || auraEffect.Aura == 89) {
		periodMs = 3000
	}
	amount := uint32(0)
	if auraEffect.BasePoints >= 0 {
		amount = uint32(auraEffect.BasePoints + 1)
	}
	if amount == 0 && (auraEffect.Aura == 3 || auraEffect.Aura == 23 || auraEffect.Aura == 89) {
		amount = uint32(10 + int(s.player.Level)*2)
	}
	schoolMask := uint8(spell.SchoolMask)
	if schoolMask == 0 {
		schoolMask = 1
	}
	object := &dynamicSpellObjectState{GUID: dynamicSpellGUID(lowGUID), CasterGUID: s.playerGUID, SpellID: uint64(spell.ID), Map: s.player.Map, InstanceID: s.player.InstanceID, X: x, Y: y, Z: z, Orientation: s.player.Orientation, Radius: radius, CastTime: uint32(time.Now().UnixMilli()), SpellData: spell, AuraEffect: auraEffect, AuraDurationMs: uint32(durationMs), AuraPeriodMs: periodMs, AuraAmount: amount, AuraSchoolMask: schoolMask, NextAuraTick: time.Now().Add(time.Duration(periodMs) * time.Millisecond)}
	s.server.spawnDynamicSpellObject(object, time.Duration(durationMs)*time.Millisecond)
}

// spellBonusMultiplier mirrors the coefficient selection in TrinityCore
// Unit::SpellDamageBonusDone (Unit.cpp:6685) and Unit::SpellHealingBonusDone
// (Unit.cpp:7562): SpellEffectInfo::BonusMultiplier = Spell.dbc
// EffectBonusCoefficient (fields 229-231). A negative DBC value falls back to
// the default (Cast Time / 3.5) coefficient, x1.88 for healing.
func (s *session) spellBonusMultiplier(spellID uint32, effIndex int, heal bool) float64 {
	mult := 0.857 // standard 3.0s cast (~85.7%)
	if s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
			coeff := -1.0
			if effIndex >= 0 && effIndex < len(spell.Effects) {
				coeff = float64(spell.Effects[effIndex].BonusCoefficient)
			}
			if coeff < 0 {
				if spell.CastingTimeIndex > 0 {
					if ct, ok, _ := s.server.Data.SpellCastTime(spell.CastingTimeIndex); ok && ct > 0 {
						coeff = float64(ct) / 3500.0
						if coeff > 1.0 {
							coeff = 1.0
						}
					}
				} else {
					coeff = 1.5 / 3.5 // instant cast coefficient ~0.4286
				}
				if heal {
					coeff *= 1.88
				}
			}
			mult = coeff
		}
	}
	return mult
}

func (s *session) executeSpellDamage(ctx context.Context, targetGUID uint64, spellID, damage uint32, effIndex int) uint32 {
	return s.executeSpellDamageWithFlags(ctx, targetGUID, spellID, damage, effIndex, false)
}

// executeSpellDamageNoCrit mirrors the SpellDamageBonusDone
// (Unit.cpp:6685) spell-power bonus and school-mask derivation of
// executeSpellDamage for the SPELL_AURA_PROC_TRIGGER_DAMAGE (43) arm, which
// never rolls hit or crit (SpellAuraEffects.cpp:5738-5762).
func (s *session) executeSpellDamageNoCrit(ctx context.Context, targetGUID uint64, spellID, damage uint32, effIndex int) uint32 {
	return s.executeSpellDamageWithFlags(ctx, targetGUID, spellID, damage, effIndex, true)
}

// executeSpellDamageWithFlags is the executeSpellDamage pipeline
// (SpellDamageBonusDone spell-power bonus via spellBonusMultiplier, school
// mask from the Spell DBC) with a procDamage leg: the
// SPELL_AURA_PROC_TRIGGER_DAMAGE (43) arm
// (AuraEffect::HandleProcTriggerDamageAuraProc, SpellAuraEffects.cpp:5738-5762)
// goes straight from SpellDamageBonusDone into the damage pipeline — never a
// hit roll, never a crit roll — so procDamage skips both.
func (s *session) executeSpellDamageWithFlags(ctx context.Context, targetGUID uint64, spellID, damage uint32, effIndex int, procDamage bool) uint32 {
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return 0
	}

	// Apply Spell Power bonus (TrinityCore Unit::SpellDamageBonusDone)
	if s.player != nil && s.player.SpellPower > 0 {
		damage += uint32(math.Round(float64(s.player.SpellPower) * s.spellBonusMultiplier(spellID, effIndex, false)))
	}

	// Use the school mask from the Spell DBC (field 17). Fallback to physical (1).
	schoolMask := uint8(1)
	if s.server != nil && s.server.Data != nil {
		if spell, found, err := s.server.Data.Spell(spellID); err == nil && found && spell.SchoolMask != 0 {
			schoolMask = uint8(spell.SchoolMask)
		}
	}

	if procDamage {
		return s.executeDirectSpellDamageNoCrit(ctx, targetGUID, spellID, damage, schoolMask)
	}
	return s.executeDirectSpellDamage(ctx, targetGUID, spellID, damage, schoolMask)
}

func (s *session) executeSpellInstantKill(ctx context.Context, targetGUID uint64, spellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return
	}
	packet := protocol.NewBuffer(20)
	packet.WriteU64(s.playerGUID)
	packet.WriteU64(targetGUID)
	packet.WriteU32(spellID)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLINSTAKILLLOG), packet.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToInstance(target.Map, target.InstanceID, uint16(protocol.OpcodeSMSG_SPELLINSTAKILLLOG), packet.Bytes(), s)
	}
	s.executeDirectSpellDamageWithFlags(ctx, targetGUID, spellID, target.Health, 1, true, false)
}

func (s *session) executeDirectSpellDamage(ctx context.Context, targetGUID uint64, spellID, damage uint32, schoolMask uint8) uint32 {
	return s.executeDirectSpellDamageWithFlags(ctx, targetGUID, spellID, damage, schoolMask, false, false)
}

// executeDirectSpellDamageNoCrit runs the direct-spell-damage pipeline with
// the hit and crit rolls disabled, for the
// SPELL_AURA_PROC_TRIGGER_DAMAGE (43) arm
// (AuraEffect::HandleProcTriggerDamageAuraProc,
// SpellAuraEffects.cpp:5738-5762), which deals SpellDamageBonusDone damage
// straight into CalculateSpellDamageTaken and the damage log — never a miss,
// never a crit.
func (s *session) executeDirectSpellDamageNoCrit(ctx context.Context, targetGUID uint64, spellID, damage uint32, schoolMask uint8) uint32 {
	return s.executeDirectSpellDamageWithFlags(ctx, targetGUID, spellID, damage, schoolMask, false, true)
}

// spellMechanicMask mirrors TrinityCore SpellInfo::GetAllEffectsMechanicMask
// (SpellInfo.cpp:1893): the spell-level Mechanic plus each active effect's
// Mechanic, ORed as 1 << mechanic.
func spellMechanicMask(spell wotlk.Spell) uint32 {
	var mask uint32
	if spell.Mechanic != 0 {
		mask |= 1 << spell.Mechanic
	}
	for _, effect := range spell.Effects {
		if effect.Effect != 0 && effect.Mechanic != 0 {
			mask |= 1 << effect.Mechanic
		}
	}
	return mask
}

// damageFromCasterMultiplier mirrors the SPELL_AURA_MOD_DAMAGE_FROM_CASTER
// arm of TrinityCore Unit::SpellDamageBonusTaken (Unit.cpp:7094): the
// victim's auras of that type multiply damage only when the aura's caster
// matches this caster and the aura spell affects the damage spell
// (AuraEffect::IsAffectedOnSpell).
func damageFromCasterMultiplier(victim, caster *session, spell wotlk.Spell) float32 {
	if victim == nil || caster == nil || victim.server == nil || victim.server.Data == nil {
		return 1
	}
	multiplier := float32(1)
	for _, aura := range victim.loadedAuras() {
		if aura == nil || aura.Stopped || aura.EffectMask == 0 || aura.CasterGUID != caster.playerGUID {
			continue
		}
		auraSpell, found, err := victim.server.Data.Spell(aura.SpellID)
		if err != nil || !found {
			continue
		}
		for index, effect := range auraSpell.Effects {
			if index >= len(aura.Amounts) || effect.Aura != spellAuraModDamageFromCaster || aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			if !spellAffectedBySpellFamilyMask(auraSpell.SpellFamilyName, effect.SpellClassMask, spell) {
				continue
			}
			amount := aura.Amounts[index]
			if amount == 0 {
				amount = int32(aura.Amount)
			}
			if amount == 0 {
				amount = effect.BasePoints + 1
			}
			multiplier *= 1 + float32(amount)/100
		}
	}
	return multiplier
}

// spellDamageBonusTaken mirrors TrinityCore Unit::SpellDamageBonusTaken
// (Unit.cpp:7052): the victim-side percent multiplier on direct spell
// damage. The DIRECT_DAMAGE (melee) early-out has no reachable arm on this
// Go path — only spell damage arrives here. The mechanic-mask term and the
// MOD_DAMAGE_PERCENT_TAKEN term read the victim's auras; the fixed-damage
// exclusion (SPELL_ATTR4_FIXED_DAMAGE) skips all but the mechanic term; the
// Sanctified Wrath bypass eats the victim's reduction through the caster's
// MOD_IGNORE_TARGET_RESIST auras. Terms with no Go model: the npcbot
// BotMgr::GetBotDamageTakenMod arm. A nil victim session fails open,
// preserving prior behavior.
func spellDamageBonusTaken(damage uint32, spell wotlk.Spell, schoolMask uint32, victim, caster *session) uint32 {
	if victim == nil {
		return damage
	}
	takenTotalMod := float32(1)
	if mechanicMask := spellMechanicMask(spell); mechanicMask != 0 {
		takenTotalMod *= ResolveAuraPercentMultiplier(victim.auraTypeModifiersByMiscMask(spellAuraModMechanicDamageTakenPercent, mechanicMask))
	}
	// Cheat Death dummy-aura arm (Unit.cpp:7075-7089): a SPELL_AURA_DUMMY
	// effect of a spell carrying SpellIconID 2109 whose effect MiscValue
	// covers the normal school reduces damage taken by AddPct (Util.h:72)
	// of the less-negative of -GetMeleeCritDamageReduction(400) and the
	// effect's amount. GetMeleeCritDamageReduction (Unit.h:970) is
	// CalculatePct(400, min(CR_CRIT_TAKEN_MELEE bonus*2.2, 33.0)) — the
	// same capped percent getResilienceStats returns as critDamageReduction,
	// and CalculatePct(400, pct) truncates to 4*pct as a whole number.
	if victim.player != nil && victim.server != nil && victim.server.Data != nil {
		_, critDmgRed, _ := getResilienceStats(victim.player.Level, victim.player.CombatRatings[CombatRatingCritTakenMelee])
		mod := -float32(uint32(4 * critDmgRed))
		for _, aura := range victim.loadedAuras() {
			if aura == nil || aura.Stopped {
				continue
			}
			cheatSpell, found, err := victim.server.Data.Spell(aura.SpellID)
			if err != nil || !found || cheatSpell.SpellIconID != spellIconCheatDeath {
				continue
			}
			for index, effect := range cheatSpell.Effects {
				if effect.Aura != spellAuraDummy || aura.EffectMask&(1<<uint(index)) == 0 || effect.MiscValue&spellSchoolMaskNormal == 0 {
					continue
				}
				amount := aura.Amounts[index]
				if amount == 0 {
					amount = int32(aura.Amount)
				}
				pct := mod
				if a := float32(amount); a > pct {
					pct = a
				}
				takenTotalMod += takenTotalMod * pct / 100
			}
		}
	}
	if spell.AttributesEx4&spellAttr4FixedDamage == 0 {
		takenTotalMod *= ResolveAuraPercentMultiplier(victim.auraTypeModifiersByMiscMask(spellAuraModDamagePercentTaken, schoolMask))
		takenTotalMod *= damageFromCasterMultiplier(victim, caster, spell)
	}
	if caster != nil && takenTotalMod < 1 {
		damageReduction := float32(1) - takenTotalMod
		for _, amount := range caster.auraTypeModifiersByMiscMask(spellAuraModIgnoreTargetResist, schoolMask) {
			damageReduction *= 1 - float32(amount)/100
		}
		takenTotalMod = 1 - damageReduction
	}
	result := float64(damage) * float64(takenTotalMod)
	if result < 0 {
		result = 0
	}
	return uint32(result)
}

// spellDamagePushesBack mirrors the pushback half of Unit::DealDamage
// (Unit.cpp:937): damage dealt by a spell carrying
// SPELL_ATTR7_NO_PUSHBACK_ON_DAMAGE or SPELL_ATTR3_TREAT_AS_PERIODIC never
// delays the victim's cast or channel, and self-inflicted damage never
// pushes back (C++ victim != attacker). A spell that fails to load degrades
// to pushback, like the C++ null-spellProto path.
func (s *session) spellDamagePushesBack(spellID uint32, victimGUID uint64) bool {
	if victimGUID == s.playerGUID {
		return false
	}
	if s.server == nil || s.server.Data == nil {
		return true
	}
	spell, found, err := s.server.Data.Spell(spellID)
	if err != nil || !found {
		return true
	}
	return spell.AttributesEx3&spellAttr3TreatAsPeriodic == 0 && spell.AttributesEx7&spellAttr7NoPushbackOnDamage == 0
}

func (s *session) executeDirectSpellDamageWithFlags(ctx context.Context, targetGUID uint64, spellID, damage uint32, schoolMask uint8, instantKill bool, procDamage bool) uint32 {
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	// DoDamageAndTriggers keys the damage arm off the incoming damage
	// (if (spell->m_damage > 0) hasDamage = true, Spell.cpp:2519): damage
	// later reduced to zero by absorb/resist stays on the damage arm; only
	// a zero incoming damage takes the no-damage arm (Spell.cpp:2563-2579).
	hadIncomingDamage := damage > 0
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return 0
	}

	isPlayerVictim := s.server != nil && s.server.findSessionByGUID(target.GUID) != nil
	if !isPlayerVictim && s.server != nil && s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, target.GUID) {
		hitInfo := uint32(0x01) // SPELL_HIT_TYPE_MISS
		damage = 0
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, 0, schoolMask, 0, 0, hitInfo), true)
		return 0
	}
	isHit := true
	crit := false
	if targetGUID != s.playerGUID && !instantKill && !procDamage {
		isHit = s.rollSpellHit(target.Level, isPlayerVictim)
	}
	hitInfo := uint32(0)
	resisted := uint32(0)
	absorbed := uint32(0)
	fullyResisted := false
	immune := false

	if !isHit {
		hitInfo = 0x01 // SPELL_HIT_TYPE_MISS
		damage = 0
	} else {
		// Spell crit roll (fixed damage backlash spells do not crit, per TrinityCore SPELL_ATTR4_FIXED_DAMAGE)
		crit = false
		spellKnown := false
		if s.server != nil && s.server.Data != nil {
			if _, found, err := s.server.Data.Spell(spellID); err == nil && found {
				spellKnown = true
			}
		}
		if !instantKill && !procDamage && spellKnown && spellID != 31117 && spellID != 64085 {
			crit = s.rollSpellCrit(target.GUID, schoolMask)
		}
		if crit {
			mult := 1.5
			if s.server != nil && s.server.Data != nil {
				if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
					mult = s.getSpellCritMultiplier(sp)
				} else {
					mult = s.getSpellCritMultiplier(wotlk.Spell{ID: spellID, SchoolMask: uint32(schoolMask)})
				}
			} else {
				mult = s.getSpellCritMultiplier(wotlk.Spell{ID: spellID, SchoolMask: uint32(schoolMask)})
			}
			damage = uint32(math.Round(float64(damage) * mult))
			hitInfo = 0x02 // SPELL_HIT_TYPE_CRIT
		}

		if !instantKill && schoolMask > 1 && s.player != nil && s.player.Level > 0 {
			chaosBolt := false
			if s.server != nil && s.server.Data != nil {
				if sp, found, err := s.server.Data.Spell(spellID); err == nil && found {
					chaosBolt = sp.SpellFamilyName == spellFamilyWarlock && sp.SpellIconID == 3178
				}
			}
			resisted, damage = calcMagicSpellResistance(damage, schoolMask, target.Resistances, s.player.Level, target.Level, !isPlayerVictim, chaosBolt, s.player.SpellPenetration)
			if resisted > 0 && damage == 0 {
				// PROC_HIT_FULL_RESIST (createProcHitMask, Unit.cpp:10179):
				// captured here, before immunity/absorption/taken
				// multipliers can zero the damage for other reasons.
				fullyResisted = true
			}
		}

		if instantKill {
			damage, hitInfo, resisted, absorbed = target.Health, 0, 0, 0
		}
		if isPlayerVictim {
			if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil {
				// Reference BE_SPELL_TARGET (28) and TIMED 6: being the
				// target of a hostile spell credits the victim and starts
				// target-timed criteria.
				playerSess.updateAchievementCriteria(criteriaTypeBeSpellTarget, spellID, 1)
				playerSess.updateAchievementCriteria(criteriaTypeBeSpellTarget2, spellID, 1)
				playerSess.startTimedAchievement(timedTypeSpellTarget, spellID)
				if !instantKill && playerSess.isImmuneToDamage(uint32(schoolMask)) {
					damage = 0
					immune = true
				}
				// Victim-side damage-taken multiplier (TrinityCore
				// Unit::SpellDamageBonusTaken, Unit.cpp:7052), applied
				// before resilience/absorption like the C++
				// EffectSchoolDMG ordering (SpellEffects.cpp:785).
				if !instantKill && damage > 0 && s.server != nil && s.server.Data != nil {
					if spell, found, err := s.server.Data.Spell(spellID); err == nil && found {
						damage = spellDamageBonusTaken(damage, spell, uint32(schoolMask), playerSess, s)
					}
				}
				isCrit := (hitInfo & 0x02) != 0 // SPELL_HIT_TYPE_CRIT
				if !instantKill {
					playerSess.applyResilienceToDamage(true, &damage, isCrit, CombatRatingCritTakenSpell)
				}
				if !instantKill && damage > 0 {
					absorbed, damage = playerSess.applyAbsorptionShields(damage, schoolMask)
				}
			}
		} else if !instantKill && s.server != nil && damage > 0 {
			absorbed, damage = s.server.applyCreatureAbsorptionShields(creatureAuraKeyForTarget(target), damage, schoolMask)
		}
	}

	overkill := uint32(0)
	if damage >= target.Health && target.Health > 0 {
		overkill = damage - target.Health
	}
	s.updateAchievementCriteria(criteriaTypeDamageDone, 0, damage)
	s.setAchievementCriteria(criteriaTypeHighestHitDealt, 0, damage)

	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, overkill, schoolMask, absorbed, resisted, hitInfo), true)

	// Trigger spell cast/hit procs (TrinityCore Unit::ProcDamageAndSpellFor);
	// suppressed for triggered casts (TRIGGERED_DISALLOW_PROC_EVENTS parity).
	// Item combat spells fire on spell hits only for melee/ranged
	// damage-class spells (Spell.cpp:2588-2596); magic-damage-class spells
	// never qualify. They also require a landed, non-immune, non-fully-
	// resisted hit: C++ evaluates the item-spell table only when canTrigger
	// holds (Player.cpp:8109), and a miss (PROC_HIT_MISS), immunity
	// (PROC_HIT_IMMUNE), or full resist (PROC_HIT_FULL_RESIST) never
	// intersects the default hit mask.
	if s.triggeredNoProcEvents == 0 && s.spellHitMayFireItemProcs(spellID) &&
		spellHitCanTriggerItemProcs(isHit, immune, fullyResisted, absorbed) {
		s.procSpellCastAndHitEffects(ctx, target, spellID)
		s.procWeaponEnchantProcsFromSpellHit(ctx, target, !(damage >= target.Health && target.Health > 0))
	}

	// Real aura procs on the spell-hit event (TrinityCore
	// Unit::ProcDamageAndSpellFor via Spell::TargetInfo::DoDamageAndTriggers,
	// Spell.cpp:2427-2579): the event carries the casting spell and the
	// triggered state so the CanSpellTriggerProcOnEvent eventSpell/triggered
	// gates engage; the triggered-cast suppression is the gate's own job,
	// not a call-site skip. A zero incoming damage takes the no-damage arm
	// (PROC_SPELL_TYPE_NO_DMG_HEAL, Spell.cpp:2563-2579), not the damage arm.
	if hadIncomingDamage {
		s.procSpellHitAuraTriggers(ctx, targetGUID, spellID, isHit, immune, fullyResisted, absorbed > 0 && damage == 0, crit, absorbed, damage)
		// Taken-side aura procs on the victim's own auras (TrinityCore
		// Unit::TriggerAurasProcOnEvent, Unit.cpp:10413-10418): the done
		// side runs first, matching the ProcSkillsAndAuras ordering
		// (Unit.cpp:5355-5366). Creature victims have no aura plumbing in
		// Go, so only online players run the taken pass; on a self-cast
		// both passes share the session, matching C++ where the done and
		// taken passes iterate the same aura list.
		if s.server != nil {
			if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil {
				playerSess.procSpellHitTakenAuraTriggers(ctx, s.playerGUID, spellID, isHit, immune, fullyResisted, absorbed > 0 && damage == 0, crit, absorbed, damage, s.triggeredNoProcEvents > 0)
			}
		}
	} else {
		s.procSpellDamageNoDmgAuraTriggers(ctx, targetGUID, spellID, isHit, immune)
		// Taken-side no-damage pass on the victim's session, after the
		// done side, matching the ProcSkillsAndAuras ordering.
		if s.server != nil {
			if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil {
				playerSess.procSpellDamageNoDmgTakenAuraTriggers(ctx, s.playerGUID, spellID, isHit, immune, s.triggeredNoProcEvents > 0)
			}
		}
	}

	s.lastCombatTime = time.Now()
	if s.player != nil && s.player.UnitFlags&unitFlagInCombat == 0 {
		s.player.UnitFlags |= unitFlagInCombat
		s.sendPlayerUpdate()
	}

	// If target is an online player (e.g. duel opponent or PvP)
	if s.server != nil {
		if playerSess := s.server.findSessionByGUID(target.GUID); playerSess != nil && playerSess.player != nil {
			playerSess.updateAchievementCriteria(criteriaTypeTotalDamageReceived, 0, damage)
			playerSess.setAchievementCriteria(criteriaTypeHighestHitReceived, 0, damage)
			playerSess.lastCombatTime = time.Now()
			if playerSess.player.UnitFlags&unitFlagInCombat == 0 {
				playerSess.player.UnitFlags |= unitFlagInCombat
			}
			_ = playerSess.write(uint16(protocol.OpcodeSMSG_SPELLNONMELEEDAMAGELOG), buildSpellNonMeleeDamageLog(target.GUID, s.playerGUID, spellID, damage, overkill, schoolMask, absorbed, resisted, hitInfo), true)
			if damage > 0 && damage >= playerSess.player.Health {
				if s.duelPartner == target.GUID && s.player.DuelTeam != 0 {
					// Duel defeat: loser drops to 1 HP and duel completes
					playerSess.player.Health = 1
					playerSess.sendPlayerUpdate()
					s.endDuel(true, s.playerGUID, false)
				} else {
					playerSess.player.Health = 0
					playerSess.sendPlayerUpdate()
					if s.server != nil {
						s.server.creditHonorableKill(s, playerSess)
					}
					playerSess.killPlayer(ctx)
					// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): C++ Unit::Kill
					// pet arm — attacker is the player, so only the
					// attacker's live pet gets KilledUnit(victim)
					// (Unit.cpp:11324-11335).
					if pet := s.livePetMotion(); pet != nil {
						s.server.fireCreatureTargetDied(ctx, pet, playerSess.luaPlayer())
					}
					s.server.handleWGPlayerDeath(playerSess, s)
				}
			} else if damage > 0 {
				playerSess.player.Health -= damage
				if s.spellDamagePushesBack(spellID, playerSess.playerGUID) {
					playerSess.delayCurrentCast()
					playerSess.delayCurrentChannel()
				}
				playerSess.procDamageAuras(true, damage)
				playerSess.sendPlayerUpdate()
			}
			return damage
		}
	}

	// Eluna CREATURE_EVENT_ON_DAMAGE_TAKEN (9): fires before damage apply;
	// handlers may rewrite damage via the second return (Unit::DealDamage,
	// Unit.cpp:697-702). Fired after the damage log, matching C++ sending
	// the log before DealDamage (Spell.cpp:2542). Skipped when damage,
	// absorb and resist are all zero, matching the C++ DealDamage early
	// return (Unit.cpp:1513). Player victims return above; the motion
	// lookup nil-guards anything else.
	if s.server != nil && (damage > 0 || absorbed > 0 || resisted > 0) {
		if motion := s.server.findCreatureMotion(s.player.Map, s.player.InstanceID, target.GUID); motion != nil {
			damage = s.server.fireCreatureDamageTaken(ctx, motion, s.luaPlayer(), damage)
		}
	}

	if damage >= target.Health {
		// Target dies
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
		if motion != nil {
			s.server.clearInstanceEncounter(motion)
			motion.Health = 0
			motion.DynamicFlags |= unitDynFlagLootable
			motion.InCombat = false
			motion.TargetGUID = 0
			motion.Moving = false
			if motion.ThreatMgr != nil {
				motion.ThreatMgr.ClearThreat()
			}
		}
		s.server.motionMu.Unlock()

		s.server.stopCreatureMotionInInstance(target.Map, target.InstanceID, target.GUID, target.X, target.Y, target.Z)
		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
			unitFieldHealth:       0,
			unitFieldDynamicFlags: 1, // UNIT_DYNFLAG_LOOTABLE
		})
		s.server.broadcastThreatClearInInstance(target.Map, target.InstanceID, target.GUID)
		_ = s.sendAttackStop(target.GUID, true)
		s.attackTarget = 0
		s.onCreatureKilled(ctx, target, nil)
		s.debug("target slain by spell", "account", s.accountName, "spell", spellID, "guid", target.GUID)
	} else {
		newHealth := target.Health - damage
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, target.GUID)
		if motion != nil {
			motion.Health = newHealth
			motion.InCombat = true
			if motion.ThreatMgr == nil {
				motion.ThreatMgr = NewThreatManager(target.GUID)
			}
			if motion.BossAI == nil {
				motion.BossAI = getBossAIForCreature(motion, motion.ScriptName)
			}
			dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
			inMelee := dist <= meleeAttackRange
			threat := float32(damage) * s.getThreatMultiplier(uint32(schoolMask))
			switched, newVictim := motion.ThreatMgr.AddThreat(s.playerGUID, threat, inMelee)
			if switched && newVictim != motion.TargetGUID {
				motion.TargetGUID = newVictim
				entries := motion.ThreatMgr.SortedEntries()
				s.server.broadcastHighestThreatUpdateInInstance(motion.Map, motion.InstanceID, motion.GUID, newVictim, entries)
			} else {
				motion.TargetGUID = motion.ThreatMgr.GetCurrentVictim()
			}
			if motion.BossAI != nil {
				motion.BossAI.OnDamageTaken(ctx, s.server, motion, s.playerGUID, damage)
			}
			motion.Moving = true
		}
		s.server.motionMu.Unlock()

		// Deferred Eluna summon hooks queued by boss OnDamageTaken (e.g.
		// VanCleef's 50% summon arm): the fire must run after the unlock
		// since Lua handler methods lock motionMu on demand.
		if motion != nil {
			s.server.drainBossSummonHooks(ctx, motion, motion.BossAI)
		}

		s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{unitFieldHealth: newHealth})
		s.server.procCreatureDamageAuras(creatureAuraKeyForTarget(target), true, damage, target.MaxHealth)
		s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
		s.server.triggerPetDefensive(s.player.Map, s.player.InstanceID, s.playerGUID, targetGUID)
	}
	return damage
}

// castSpellDirect triggers an immediate, instant cast of a spell without cast time or resource cost.
// Mirrors TrinityCore Unit::CastSpell (Spell.cpp: triggered = true).
func (s *session) castSpellDirect(ctx context.Context, spellID uint32, targetGUID uint64) {
	s.castSpellDirectWithOptions(ctx, spellID, targetGUID, false)
}

func (s *session) castFirstLoginSpell(ctx context.Context, spellID uint32, targetGUID uint64) {
	s.castSpellDirectWithOptions(ctx, spellID, targetGUID, true)
}

func (s *session) castSpellDirectWithOptions(ctx context.Context, spellID uint32, targetGUID uint64, firstLogin bool) {
	s.castSpellDirectWithOverrides(ctx, spellID, targetGUID, firstLogin, nil)
}

func (s *session) castSpellDirectWithBasePoint(ctx context.Context, spellID uint32, targetGUID uint64, basePoint uint32) {
	value := int32(basePoint)
	s.castSpellDirectWithOverrides(ctx, spellID, targetGUID, false, &value)
}

func (s *session) castSpellDirectWithOverrides(ctx context.Context, spellID uint32, targetGUID uint64, firstLogin bool, basePoint0 *int32) {
	if s == nil || s.player == nil || spellID == 0 {
		return
	}
	// C++ Unit::CastSpell(id, true) runs Spell::cast, whose re-entrant
	// wrapper (Spell.cpp:3266-3280) pushes a fresh taking spell for the
	// nested cast and restores the outer one after; the context stack
	// reproduces that, so triggered casts register their mods on their
	// own context.
	s.beginSpellModTaking()
	defer s.endSpellModTaking()
	if ctx == nil {
		ctx = context.Background()
	}

	// Triggered casts map to C++ Unit::CastSpell(..., triggered=true), which sets
	// TRIGGERED_FULL_MASK including TRIGGERED_DISALLOW_PROC_EVENTS
	// (SpellDefines.h:155). C++ suppresses the attacker's own proc rolls for such
	// spells (Unit::TriggerAurasProcOnEvent, Unit.cpp:10424, Spell::IsProcDisabled).
	s.triggeredNoProcEvents++
	defer func() { s.triggeredNoProcEvents-- }()

	var spell wotlk.Spell
	found := false
	if s.server != nil && s.server.Data != nil {
		var err error
		spell, found, err = s.server.Data.Spell(spellID)
		if err != nil || !found {
			found = false
		}
	}
	if !found {
		spell = wotlk.Spell{ID: spellID}
	}
	if basePoint0 != nil && len(spell.Effects) != 0 {
		spell.Effects[0].BasePoints = *basePoint0 - 1
	}

	castID := uint8(0) // C++ m_cast_count stays 0 for triggered casts (Spell.cpp:602; set nonzero only for client-initiated casts, Player.cpp:8251)
	now := time.Now()
	castTimeStamp := uint32(now.UnixMilli())
	if firstLogin {
		castTimeStamp = gameTimeMS()
	}
	hitTargets := []uint64{targetGUID}
	spellTargetFlags := protocol.SpellTargetFlagUnitWireMask
	if firstLogin {
		spellTargetFlags = protocol.SpellTargetFlagUnit
	}
	spellTarget := protocol.SpellTargetData{Flags: spellTargetFlags, UnitGUID: targetGUID}
	// Spell::SelectImplicitChannelTargets (Spell.cpp:980-1032): a triggered
	// spell with channel-dest implicit targets (76/106) resolves its
	// destination from the currently channeled spell, so the SMSG_SPELL_GO
	// spell-target block and dest-consuming read sites see it. Nothing is
	// resolved when no channel is live (the C++ null gate).
	if x, y, z, ok := s.channelDestForSpell(ctx, spell); ok {
		spellTarget.Flags |= protocol.SpellTargetFlagDestLocation
		spellTarget.Destination = protocol.SpellTargetLocation{X: x, Y: y, Z: z}
	}
	// Cast flags mirror Spell::SendSpellGo for a triggered player cast
	// (Spell.cpp:4283-4330): PENDING for triggered non-auto-repeat casts with
	// cast count 0 (Spell.cpp:4292), POWER_LEFT_SELF + remaining power for
	// non-health powers (Spell.cpp:4297), NO_GCD when the spell has no start
	// recovery time (Spell.cpp:4325). RUNE_LIST never applies to triggered
	// casts (FULL_MASK carries TRIGGERED_IGNORE_POWER_AND_REAGENT_COST).
	// AMMO (Spell.cpp:4295) is skipped: Go has no ammo display data, and the
	// flag without the 8-byte Ammo block would corrupt the packet.
	castFlags := uint32(spellCastFlagGo)
	if spell.AttributesEx1&spellAttr2AutorepeatFlag == 0 {
		castFlags |= spellCastFlagPending
	}
	var remainingPower *uint32
	if spell.PowerType != 0xFFFFFFFE /* POWER_HEALTH = -2 in C++ */ && int(spell.PowerType) < len(s.player.Powers) {
		castFlags |= protocol.SpellCastFlagPowerLeftSelf
		power := s.player.Powers[spell.PowerType]
		remainingPower = &power
	}
	if spell.StartRecoveryTime == 0 {
		castFlags |= protocol.SpellCastFlagNoGCD
	}
	// Spell::IsNeedSendToClient (Spell.cpp:7543): triggered casts only send
	// SMSG_SPELL_GO when the spell has a visual, is channeled, or has speed.
	if spell.SpellVisual[0] != 0 || spell.SpellVisual[1] != 0 || isChanneledSpell(spell) || spell.Speed > 0 {
		goPkt := protocol.BuildSpellGoWithPower(s.playerGUID, s.playerGUID, castID, spellID, castFlags, castTimeStamp, hitTargets, nil, spellTarget, remainingPower)
		_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), goPkt, true)
		if s.server != nil {
			// C++ sends the caster a self-only packet carrying POWER_LEFT_SELF
			// and re-broadcasts to the set with the flag (and power) stripped
			// (Spell.cpp:4353-4366).
			nearbyPacket := goPkt
			if castFlags&protocol.SpellCastFlagPowerLeftSelf != 0 {
				nearbyPacket = protocol.BuildSpellGo(s.playerGUID, s.playerGUID, castID, spellID, castFlags&^protocol.SpellCastFlagPowerLeftSelf, castTimeStamp, hitTargets, nil, spellTarget)
			}
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_GO), nearbyPacket, s)
		}
	}

	// Spell::_handle_immediate_phase (Spell.cpp:3718): initial spell threat
	// (HandleThreatSpells, Spell.cpp:5096) applies to triggered casts too,
	// before any effect handling.
	s.handleSpellInitialThreat(ctx, spell, hitTargets)

	durationMs := uint32(0)
	if s.server != nil && s.server.Data != nil && spell.DurationIndex > 0 {
		lvl := uint32(80)
		if s.player != nil && s.player.Level > 0 {
			lvl = uint32(s.player.Level)
		}
		if dur, ok, err := s.server.Data.SpellDuration(spell.DurationIndex, lvl); err == nil && ok && dur > 0 {
			durationMs = uint32(dur)
		}
	}
	if durationMs == 0 {
		switch spellID {
		case ProcSpellBerserking:
			durationMs = 12000
		case ProcSpellMongoose:
			durationMs = 15000
		case ProcSpellExecutioner:
			durationMs = 15000
		case ProcSpellCrusader:
			durationMs = 15000
		case ProcSpellCrippling:
			durationMs = 12000
		case ProcSpellDeadlyPois:
			durationMs = 12000
		case ProcSpellWoundPois:
			durationMs = 15000
		}
	}

	hasExplicitEffects := false
	// Per-(cast, target) first-merge marker for the aura re-apply path
	// (Spell.cpp:2842): only the first aura effect per target runs the
	// ModStackAmount(+1) merge, later effects only refresh their amounts.
	castMerged := make(map[uint64]struct{})
	for effectIndex, eff := range spell.Effects {
		if eff.Effect == 0 && eff.Aura == 0 {
			continue
		}
		hasExplicitEffects = true
		if eff.Effect == 1 { // SPELL_EFFECT_INSTAKILL
			s.executeSpellInstantKill(ctx, targetGUID, spellID)
		} else if eff.Effect == 2 { // SPELL_EFFECT_SCHOOL_DAMAGE
			baseDmg := uint32(eff.BasePoints + 1)
			if baseDmg == 0 {
				if spellID == ProcSpellFieryWeapon {
					baseDmg = 40
				} else if spellID == ProcSpellInstantPois {
					baseDmg = 280
				}
			}
			s.executeDirectSpellDamage(ctx, targetGUID, spellID, baseDmg, uint8(spell.SchoolMask))
		} else if eff.Effect == 6 || eff.Aura != 0 { // SPELL_EFFECT_APPLY_AURA
			amount := uint32(eff.BasePoints + 1)
			schoolMask := spell.SchoolMask
			if schoolMask == 0 {
				schoolMask = 1
			}
			s.applyAuraToTarget(ctx, targetGUID, spell, eff, durationMs, eff.AuraPeriod, amount, schoolMask, castMerged, false, s.playerGUID)
		} else if eff.Effect == 10 { // SPELL_EFFECT_HEAL
			healAmount := uint32(eff.BasePoints + 1)
			if healAmount == 0 && spellID == ProcSpellCrusader {
				healAmount = 100
			}
			s.executeSpellHeal(ctx, targetGUID, spellID, healAmount, effectIndex)
		} else if eff.Effect == spellEffectEnergize {
			s.applySpellEnergize(ctx, targetGUID, eff.MiscValue, eff.BasePoints+1)
		} else if eff.Effect == spellEffectPowerBurn {
			if burned := s.applySpellPowerBurn(ctx, targetGUID, eff.MiscValue, eff.BasePoints+1, spellID); burned > 0 {
				s.executeSpellDamage(ctx, targetGUID, spellID, effectValueMultiplied(burned, eff.Amplitude), effectIndex)
			}
		} else if eff.Effect == spellEffectTriggerSpell {
			if eff.TriggerSpell != 0 && eff.TriggerSpell != spellID {
				s.castSpellDirect(ctx, eff.TriggerSpell, targetGUID)
			}
		} else if eff.Effect == spellEffectThreat {
			s.applySpellThreat(ctx, targetGUID, eff.BasePoints+1)
		} else if eff.Effect == spellEffectHealMaxHealth {
			s.executeSpellMaxHealthHeal(ctx, targetGUID, spellID)
		}
	}

	if !hasExplicitEffects {
		eff := wotlk.SpellEffect{Effect: 6, Aura: 4}
		s.applyAuraToTarget(ctx, targetGUID, spell, eff, durationMs, 0, 0, 1, nil, false, s.playerGUID)
	}

	// Spell::_cast (Spell.cpp:3502-3511): a triggered cast (C++
	// Unit::CastSpell(id, true)) runs the same _cast tail, so the
	// spell_linked_spell list fires here too. The CHEAT_COOLDOWN leg
	// (Spell.cpp:3517-3523) runs on this tail as well. The PHASE_CAST
	// proc leg (Spell.cpp:3525-3545) runs on this tail as well; the
	// CanSpellTriggerProcOnEvent gate suppresses events whose triggered
	// flag bars proccing, mirroring C++.
	s.fireSpellLinkedTriggers(ctx, spellID, targetGUID)
	s.resetCastCooldownCheat(spellID)
	s.procSpellCastPhaseAuraTriggers(ctx, spell)
}

func (s *session) applySpellEnergize(ctx context.Context, targetGUID uint64, powerType int32, amount int32) {
	s.adjustSpellPower(ctx, targetGUID, powerType, int64(amount))
}

// effectValueMultiplied applies the effect's value multiplier
// (Spell.dbc EffectAmplitude, SpellInfo.cpp:344) the way
// SpellEffectInfo::CalcValueMultiplier does: raw multiplication, no
// zero default. The SPELLMOD_VALUE_MULTIPLIER term has no Go infra.
func effectValueMultiplied(value uint32, amplitude float32) uint32 {
	return uint32(int32(float64(value) * float64(amplitude)))
}

// drainManaResilienceReduction mirrors the resilience term in
// Spell::EffectPowerDrain and Spell::EffectPowerBurn
// (SpellEffects.cpp:1285-1287): mana drains are reduced by the target's
// spell crit damage reduction, i.e. the min(resiliencePct*2.2, 33.0)%
// slice of getResilienceStats. C++'s GetCombatRatingDamageReduction
// returns 0 for non-players, and Go's creature motions carry no combat
// ratings, so the term only fires on player targets.
func drainManaResilienceReduction(target *session, amount uint32) uint32 {
	if target == nil || target.player == nil || amount == 0 {
		return 0
	}
	if int(CombatRatingCritTakenSpell) >= len(target.player.CombatRatings) {
		return 0
	}
	rating := target.player.CombatRatings[CombatRatingCritTakenSpell]
	if rating == 0 || target.player.Level == 0 {
		return 0
	}
	_, critDmgRed, _ := getResilienceStats(target.player.Level, rating)
	if critDmgRed <= 0 {
		return 0
	}
	// Unit::GetCombatRatingDamageReduction = CalculatePct(damage, pct),
	// which truncates for uint32.
	reduction := uint32(float64(amount) * float64(critDmgRed) / 100.0)
	if reduction >= amount {
		return amount
	}
	return reduction
}

func (s *session) applySpellPowerBurn(ctx context.Context, targetGUID uint64, powerType int32, amount int32, spellID uint32) uint32 {
	if s == nil || s.player == nil || powerType < 0 || powerType >= 7 || amount <= 0 {
		return 0
	}
	target := s.spellPowerTarget(targetGUID)
	if target == nil {
		// SpellEffects.cpp:1265-1282 (EffectPowerDrain) and :1352-1369
		// (EffectPowerBurn): the power terms fire on any alive unit whose
		// power type matches, not only players. Creature motions carry a
		// power model (Powers/MaxPowers/PowerType); motions without
		// populated max powers stay a no-op via the maximum != 0 gate.
		return s.applySpellPowerBurnToCreature(targetGUID, powerType, amount, spellID)
	}
	if target.player == nil || target.player.Health == 0 || playerPowerType(target.player) != uint8(powerType) {
		return 0
	}
	maximum := target.player.MaxPowers[uint32(powerType)]
	if maximum == 0 {
		return 0
	}
	burn := int64(amount)
	if spellID == 8129 {
		burn = int64(maximum) * burn / 100
		casterMax := s.player.MaxPowers[uint32(powerType)]
		cap := int64(casterMax) * int64(amount) * 2 / 100
		if burn > cap {
			burn = cap
		}
	}
	if burn <= 0 {
		return 0
	}
	// SpellEffects.cpp:1285-1287 (EffectPowerDrain) and the matching
	// EffectPowerBurn term: resilience reduces mana drains by the
	// target's spell crit damage reduction (added in 2.4). This is the
	// crit-damage slice only — not the flat resilience damage reduction
	// that applyResilienceToDamage applies to real damage.
	if powerType == 0 { // POWER_MANA
		burn -= int64(drainManaResilienceReduction(target, uint32(burn)))
	}
	if burn <= 0 {
		return 0
	}
	if burn > int64(target.player.Powers[uint32(powerType)]) {
		burn = int64(target.player.Powers[uint32(powerType)])
	}
	if burn <= 0 {
		return 0
	}
	target.adjustSpellPower(ctx, targetGUID, powerType, -burn)
	return uint32(burn)
}

// applySpellPowerBurnToCreature mirrors the creature-target arm of
// Spell::EffectPowerDrain/EffectPowerBurn: an alive unit whose power type
// matches loses up to burn power, and the drained amount is returned for
// the caster-gain (drain) or damage (burn) follow-ons. The Mana Burn 8129
// target/caster cap runs against the motion's max powers; the resilience
// term is dead here because C++'s GetCombatRatingDamageReduction returns
// 0 for non-players (see drainManaResilienceReduction).
func (s *session) applySpellPowerBurnToCreature(targetGUID uint64, powerType int32, amount int32, spellID uint32) uint32 {
	if s == nil || s.player == nil || s.server == nil || targetGUID == 0 {
		return 0
	}
	index := uint32(powerType)
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	if motion == nil || motion.Health == 0 || motion.PowerType != index || motion.MaxPowers[index] == 0 {
		s.server.motionMu.Unlock()
		return 0
	}
	burn := int64(amount)
	if spellID == 8129 { // Mana Burn: burn x% of target's mana, capped at 2x% of caster's
		burn = int64(motion.MaxPowers[index]) * burn / 100
		cap := int64(s.player.MaxPowers[index]) * int64(amount) * 2 / 100
		if burn > cap {
			burn = cap
		}
	}
	if burn > int64(motion.Powers[index]) {
		burn = int64(motion.Powers[index])
	}
	if burn <= 0 {
		s.server.motionMu.Unlock()
		return 0
	}
	next := motion.Powers[index] - uint32(burn)
	motion.Powers[index] = next
	if index == 0 {
		motion.Mana = next
	} else if index == 4 {
		motion.Happiness = next
	}
	mapID, instanceID, guid := motion.Map, motion.InstanceID, motion.GUID
	s.server.motionMu.Unlock()
	s.server.broadcastCreatureValuesUpdateInInstance(mapID, instanceID, guid, map[int]uint32{unitFieldPower1 + int(index): next})
	return uint32(burn)
}

func (s *session) spellPowerTarget(targetGUID uint64) *session {
	if s == nil || s.player == nil {
		return nil
	}
	if targetGUID == 0 || targetGUID == s.playerGUID || s.server == nil {
		return s
	}
	return s.server.findSessionByGUID(targetGUID)
}

func (s *session) adjustSpellPower(ctx context.Context, targetGUID uint64, powerType int32, delta int64) {
	if s == nil || s.player == nil || powerType < 0 || powerType >= 7 || delta == 0 {
		return
	}
	if s.server != nil && targetGUID != 0 && targetGUID != s.playerGUID {
		s.server.motionMu.Lock()
		motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
		if motion != nil && motion.PetID != 0 && motion.Health > 0 {
			index := uint32(powerType)
			maximum, old := motion.MaxPowers[index], motion.Powers[index]
			if maximum != 0 {
				var next uint32
				if delta < 0 {
					drain := uint64(-(delta + 1)) + 1
					if drain >= uint64(old) {
						next = 0
					} else {
						next = old - uint32(drain)
					}
				} else if delta >= int64(maximum-old) {
					next = maximum
				} else {
					next = old + uint32(delta)
				}
				if next != old {
					motion.Powers[index] = next
					if index == 0 {
						motion.Mana = next
					} else if index == 4 {
						motion.Happiness = next
					}
					mapID, petGUID := motion.Map, motion.GUID
					s.server.motionMu.Unlock()
					s.server.broadcastCreatureValuesUpdateInInstance(mapID, motion.InstanceID, petGUID, map[int]uint32{unitFieldPower1 + int(index): next})
					return
				}
			}
		}
		s.server.motionMu.Unlock()
	}
	target := s.spellPowerTarget(targetGUID)
	if target == nil || target.player == nil || target.player.Health == 0 {
		return
	}
	index := uint32(powerType)
	maximum := target.player.MaxPowers[index]
	if maximum == 0 {
		return
	}
	old := target.player.Powers[index]
	var newPower uint32
	if delta < 0 {
		drain := uint64(-delta)
		if drain >= uint64(old) {
			newPower = 0
		} else {
			newPower = old - uint32(drain)
		}
	} else {
		newPower = old + uint32(delta)
		if newPower < old || newPower > maximum {
			newPower = maximum
		}
	}
	if newPower == old {
		return
	}
	target.player.Powers[index] = newPower
	packet := protocol.NewBuffer(13)
	packet.WritePackedGUID(target.playerGUID)
	packet.WriteU8(uint8(powerType))
	packet.WriteU32(newPower)
	_ = target.write(uint16(protocol.OpcodeSMSG_POWER_UPDATE), packet.Bytes(), true)
	if target.server != nil {
		target.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_POWER_UPDATE), packet.Bytes(), target)
		if target.server.CharactersStore != nil && target.server.CharactersStore.DB != nil {
			col := fmt.Sprintf("power%d", index+1)
			_, _ = target.server.CharactersStore.DB.ExecContext(ctx, fmt.Sprintf("UPDATE characters SET %s = ? WHERE guid = ?", col), newPower, target.playerGUID)
		}
	}
}

func (s *session) applySpellThreat(ctx context.Context, targetGUID uint64, amount int32) {
	if s == nil || s.player == nil || s.server == nil || targetGUID == 0 || amount <= 0 {
		return
	}
	s.server.motionMu.Lock()
	motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, targetGUID)
	if motion == nil || motion.Health == 0 || isCreaturePassive(motion) {
		s.server.motionMu.Unlock()
		return
	}
	if motion.ThreatMgr == nil {
		motion.ThreatMgr = NewThreatManager(motion.GUID)
	}
	wasInCombat := motion.InCombat
	inMelee := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z) <= meleeAttackRange
	switched, victim := motion.ThreatMgr.AddThreat(s.playerGUID, float32(amount), inMelee)
	motion.TargetGUID = victim
	motion.InCombat = true
	motion.Moving = false
	mapID := motion.Map
	entries := motion.ThreatMgr.SortedEntries()
	guid := motion.GUID
	s.server.motionMu.Unlock()
	if switched {
		s.server.broadcastHighestThreatUpdateInInstance(mapID, s.player.InstanceID, guid, victim, entries)
	}
	if !wasInCombat {
		s.server.broadcastAIReactionInInstance(mapID, s.player.InstanceID, guid, 2)
		startPkt := buildAttackStart(guid, s.playerGUID)
		_ = s.write(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_ATTACK_START), startPkt, s)
	}
	_ = ctx
}

func (s *session) executeSpellMaxHealthHeal(ctx context.Context, targetGUID uint64, spellID uint32) {
	if s == nil || s.player == nil {
		return
	}
	if targetGUID == 0 || targetGUID == s.playerGUID {
		if s.player.Health == 0 {
			return
		}
		s.player.Health = s.player.MaxHealth
		s.sendPlayerUpdate()
		return
	}
	if s.server == nil {
		return
	}
	if target := s.server.findSessionByGUID(targetGUID); target != nil && target.player != nil && target.player.Health > 0 {
		target.player.Health = target.player.MaxHealth
		target.sendPlayerUpdate()
		return
	}
	if creature, ok := s.getCombatTarget(ctx, targetGUID); ok && creature.Health > 0 {
		s.executeSpellHeal(ctx, targetGUID, spellID, creature.MaxHealth, 0)
	}
}

func (s *session) executeSpellHeal(ctx context.Context, targetGUID uint64, spellID, heal uint32, effIndex int) {
	if s.player == nil {
		return
	}
	if targetGUID == 0 {
		targetGUID = s.playerGUID
	}
	// The C++ heal proc arm keys off m_healing > 0 (Spell.cpp:2496), the
	// pre-spell-power amount; capture it before the bonuses below mutate heal.
	rawHeal := heal

	targetSess := s
	if targetGUID != s.playerGUID && s.server != nil {
		if other := s.server.findSessionByGUID(targetGUID); other != nil && other.player != nil {
			targetSess = other
			// Reference BE_SPELL_TARGET (28) and TIMED 6: being the target of a
			// spell credits the victim/target and starts target-timed criteria.
			other.updateAchievementCriteria(criteriaTypeBeSpellTarget, spellID, 1)
			other.updateAchievementCriteria(criteriaTypeBeSpellTarget2, spellID, 1)
			other.startTimedAchievement(timedTypeSpellTarget, spellID)
		}
	}

	// Apply Spell Power bonus to healing (TrinityCore Unit::SpellHealingBonusDone)
	if s.player != nil && s.player.SpellPower > 0 {
		heal += uint32(math.Round(float64(s.player.SpellPower) * s.spellBonusMultiplier(spellID, effIndex, true)))
	}

	// Roll healing critical strike (TrinityCore: 150% healing on crit, modified by metagem)
	isCrit := s.rollSpellCrit(0, 2)
	if isCrit {
		mult := s.getSpellCritMultiplier(wotlk.Spell{ID: spellID, SchoolMask: 2})
		heal = uint32(math.Round(float64(heal) * mult))
	}

	effectiveHeal := heal
	if targetSess.player.Health+heal > targetSess.player.MaxHealth {
		effectiveHeal = targetSess.player.MaxHealth - targetSess.player.Health
		targetSess.player.Health = targetSess.player.MaxHealth
	} else {
		targetSess.player.Health += heal
	}
	s.updateAchievementCriteria(criteriaTypeHealingDone, 0, heal)
	s.setAchievementCriteria(criteriaTypeHighestHealCasted, 0, heal)
	targetSess.updateAchievementCriteria(criteriaTypeTotalHealingReceived, 0, effectiveHeal)
	targetSess.setAchievementCriteria(criteriaTypeHighestHealingRecv, 0, heal)
	overheal := heal - effectiveHeal
	if s.server != nil {
		s.server.updateArenaHealingScore(s, effectiveHeal)
	}

	// TC Unit.cpp:6550: packet = packed(target), packed(healer), spellID, heal, overheal, absorb, crit, unused
	healPkt := buildSpellHealLog(targetGUID, s.playerGUID, spellID, heal, overheal, 0, isCrit)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), healPkt, true)
	if targetSess != s {
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_SPELLHEALLOG), healPkt, true)
	}
	targetSess.sendPlayerUpdate()
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		_, _ = s.server.CharactersStore.DB.ExecContext(ctx, "UPDATE characters SET health = ? WHERE guid = ?", targetSess.player.Health, targetSess.playerGUID)
	}

	if s.server != nil && effectiveHeal > 0 {
		s.server.distributeHealingThreat(ctx, s.playerGUID, targetGUID, effectiveHeal)
	}

	// Real aura procs on the done side of a direct heal (TrinityCore
	// Unit::ProcDamageAndSpellFor via Spell::TargetInfo::DoDamageAndTriggers,
	// Spell.cpp:2493-2513, 2581-2586): C++ applies the heal (HealBySpell) and
	// forwards the assist threat before the trigger pass, so this runs last.
	// A zero heal does not take the heal arm: C++'s no-damage arm
	// (Spell.cpp:2563-2579) runs the trigger pass with
	// PROC_SPELL_TYPE_NO_DMG_HEAL instead, on both sides.
	if rawHeal > 0 {
		s.procSpellHealAuraTriggers(ctx, targetGUID, spellID, rawHeal, isCrit)

		// Real aura procs on the taken side of a direct heal (TrinityCore
		// Unit::ProcDamageAndSpellFor via Spell::TargetInfo::DoDamageAndTriggers,
		// Spell.cpp:2462-2473, 2581-2586): the heal target's
		// TAKEN_SPELL_*_DMG_CLASS_POS auras gate against the taken-side
		// positivity-fallback mask. Runs on the target's session so its own auras
		// gate; on a self-heal this is the caster session, matching C++ where
		// ProcDamageAndSpellFor's done and taken passes iterate the same aura
		// list. The trigger spell targets the healer.
		targetSess.procSpellHealTakenAuraTriggers(ctx, s.playerGUID, spellID, rawHeal, isCrit)
	} else {
		// No-damage arm (Spell.cpp:2563-2579, 2581-2586): done side first,
		// then the taken side on the target's session, matching the
		// ProcSkillsAndAuras ordering.
		s.procSpellNoDmgHealAuraTriggers(ctx, targetGUID, spellID)
		targetSess.procSpellNoDmgHealTakenAuraTriggers(ctx, s.playerGUID, spellID)

		// Item combat spells also fire on the no-damage arm for melee/ranged
		// damage-class spells (Spell.cpp:2589-2596); heal spells are magic
		// class and fail the gate, matching C++.
		if s.triggeredNoProcEvents == 0 && s.spellHitMayFireItemProcs(spellID) {
			s.procSpellCastAndHitEffects(ctx, combatTarget{GUID: targetGUID}, spellID)
			alive := targetSess != nil && targetSess.player != nil && targetSess.player.Health > 0
			s.procWeaponEnchantProcsFromSpellHit(ctx, combatTarget{GUID: targetGUID}, alive)
		}
	}
}

func buildSpellNonMeleeDamageLog(targetGUID, attackerGUID uint64, spellID, damage, overkill uint32, schoolMask uint8, extra ...uint32) []byte {
	// Layout matches TrinityCore Unit::SendSpellNonMeleeDamageLog (Unit.cpp:5302)
	// packed target, packed attacker, spellID, damage, overkill, schoolMask,
	// absorbed, resist, periodicLog, unused, blocked, HitInfo, HitInfo&debugMask
	absorb := uint32(0)
	resist := uint32(0)
	hitInfo := uint32(0)
	if len(extra) > 0 {
		absorb = extra[0]
	}
	if len(extra) > 1 {
		resist = extra[1]
	}
	if len(extra) > 2 {
		hitInfo = extra[2]
	}
	buf := protocol.NewBuffer(64)
	buf.WritePackedGUID(targetGUID)
	buf.WritePackedGUID(attackerGUID)
	buf.WriteU32(spellID)
	buf.WriteU32(damage)
	buf.WriteU32(overkill)
	buf.WriteU8(schoolMask)
	buf.WriteU32(absorb)  // Absorbed
	buf.WriteU32(resist)  // Resist
	buf.WriteU8(0)        // periodicLog (0 = show spell name prefix)
	buf.WriteU8(0)        // unused
	buf.WriteU32(0)       // blocked
	buf.WriteU32(hitInfo) // HitInfo flags (0 = normal hit, 2 = SPELL_HIT_TYPE_CRIT)
	buf.WriteU8(0)        // HitInfo & debugMask (always 0, no crit/hit debug)
	return buf.Bytes()
}

func buildSpellHealLog(targetGUID, healerGUID uint64, spellID, healAmount, overheal, absorb uint32, crit bool) []byte {
	buf := protocol.NewBuffer(32)
	buf.WritePackedGUID(targetGUID)
	buf.WritePackedGUID(healerGUID)
	buf.WriteU32(spellID)
	buf.WriteU32(healAmount)
	buf.WriteU32(overheal)
	buf.WriteU32(absorb)
	if crit {
		buf.WriteU8(1)
	} else {
		buf.WriteU8(0)
	}
	buf.WriteU8(0)
	return buf.Bytes()
}

func (s *session) hasActiveSpell(spellID uint32) bool {
	for _, spell := range s.player.Spells {
		if spell.ID == spellID {
			return spell.Active && !spell.Disabled
		}
	}
	if s.player != nil && s.player.ShapeshiftForm != 0 && s.server != nil && s.server.Data != nil {
		if form, found, err := s.server.Data.ShapeshiftForm(uint32(s.player.ShapeshiftForm)); err == nil && found {
			for _, presetSpell := range form.PresetSpellIDs {
				if presetSpell == spellID {
					return true
				}
			}
		}
	}
	return false
}

func canPlayerCastSpell(learned, gmMode bool) bool {
	return learned || gmMode
}

func spellCastIgnoreReason(spell wotlk.Spell, found, learned bool) string {
	if !found {
		return "unknown spell"
	}
	if spell.Attributes&spellAttributePassive != 0 {
		return "passive spell"
	}
	if !learned {
		return "spell not learned"
	}
	return "unavailable"
}

// sendInterrupted mirrors C++ Spell::SendInterrupted (Spell.cpp:4624): after
// the caster receives its own SMSG_CAST_FAILED, SMSG_SPELL_FAILURE and
// SMSG_SPELL_FAILED_OTHER are broadcast to the set with the packed caster GUID.
func (s *session) sendInterrupted(castID uint8, spellID uint32, result uint8) {
	if s.server == nil || s.player == nil {
		return
	}
	payload := protocol.BuildSpellFailure(s.playerGUID, castID, spellID, result)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_FAILURE), payload, s)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_SPELL_FAILED_OTHER), payload, s)
}

func (s *session) interruptCurrentCast() {
	s.castMu.Lock()
	if s.activeCast != nil {
		if s.activeCast.Timer != nil {
			s.activeCast.Timer.Stop()
		}
		s.activeCast.Cancelled = true
		castID := s.activeCast.CastID
		spellID := s.activeCast.SpellID
		s.activeCast = nil
		// Spell::cancel (Spell.cpp:3210-3225) calls CancelGlobalCooldown() when
		// cancelled in SPELL_STATE_PREPARING.
		s.castMu.Unlock()
		s.cancelGlobalCooldown(spellID)

		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedInterrupted), true)
		s.sendInterrupted(castID, spellID, spellFailedInterrupted)
		return
	}
	s.castMu.Unlock()
}

// interruptSpellsOnMovement breaks the active cast and channel when the
// player starts moving, for spells carrying SPELL_INTERRUPT_FLAG_MOVEMENT.
// C++ authority: Unit::InterruptNonMeleeSpells via the movement interrupt
// mask (Unit.cpp), called from Spell::update's per-tick movement leg
// (Spell.cpp:3814-3831). No-bridge legs from that leg, noted:
//   - SPELL_EFFECT_STUCK spells are exempt while the caster is falling far;
//     Go has no STUCK model and checks the flag alone.
//   - IsNextMeleeSwingSpell / IsAutoRepeat / IsTriggered exclusions: Go
//     casts are always player-session casts; triggered casts route through
//     the same activeCastState, so they break on movement here where C++
//     would let them continue. Auto-repeat lives on the session
//     (autoRepeatSpell), not on a per-cast Spell object.
//   - IsMoveAllowedChannel channeled exemption: covered at startChannel —
//     Go breaks channels on movement unconditionally.
//   - the charmer-is-creature trust hack: Go has no charmed-caster model.
func (s *session) interruptSpellsOnMovement() {
	if s == nil {
		return
	}
	s.castMu.Lock()
	cast := s.activeCast
	channel := s.activeChannel
	castBreaks := cast != nil && !cast.Cancelled && cast.InterruptFlg&spellInterruptFlagMovement != 0
	channelBreaks := channel != nil && !channel.Stopped && channel.Spell.InterruptFlags&spellInterruptFlagMovement != 0
	s.castMu.Unlock()

	if castBreaks {
		s.interruptCurrentCast()
	}
	if channelBreaks {
		s.interruptCurrentChannel()
	}
}

func (s *session) stopSpellLifecycle() {
	if s == nil {
		return
	}
	s.castMu.Lock()
	if s.activeCast != nil {
		if s.activeCast.Timer != nil {
			s.activeCast.Timer.Stop()
		}
		s.activeCast.Cancelled = true
		s.activeCast = nil
	}
	channel := s.activeChannel
	if channel != nil {
		if channel.Timer != nil {
			channel.Timer.Stop()
		}
		if channel.TickTimer != nil {
			channel.TickTimer.Stop()
		}
		channel.Stopped = true
		s.activeChannel = nil
	}
	s.castMu.Unlock()
}

// handleCancelCast mirrors WorldSession::HandleCancelCastOpcode
// (SpellHandler.cpp:449-453) -> Unit::InterruptNonMeleeSpells(false, SpellID,
// false) (Unit.cpp:3212-3222) -> Spell::cancel (Spell.cpp:3210-3254) for the
// SPELL_STATE_PREPARING leg: the cancel only lands when the active cast is the
// spell the client named, the interrupted spell's global cooldown is refunded
// (CancelGlobalCooldown), and both the caster result and the set-wide
// interrupted broadcast go out (SendCastResult + SendInterrupted).
func (s *session) handleCancelCast(payload []byte) bool {
	reader := protocol.NewReader(payload)
	castID, _ := reader.ReadU8()
	spellID, _ := reader.ReadU32()

	s.castMu.Lock()
	if s.activeCast != nil && s.activeCast.SpellID == spellID {
		if s.activeCast.Timer != nil {
			s.activeCast.Timer.Stop()
		}
		s.activeCast.Cancelled = true
		curCastID := s.activeCast.CastID
		curSpellID := s.activeCast.SpellID
		s.activeCast = nil
		s.castMu.Unlock()

		s.cancelGlobalCooldown(curSpellID)
		_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(curCastID, curSpellID, spellFailedInterrupted), true)
		s.sendInterrupted(curCastID, curSpellID, spellFailedInterrupted)
		return true
	}
	s.castMu.Unlock()

	// No active cast (or a different spell is casting): C++ sends nothing, but
	// the cast-failed result still goes out so a client stuck on a cast bar
	// for this spell id can clear it.
	_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(castID, spellID, spellFailedInterrupted), true)
	return true
}

// Spell::cancel parity (Spell.cpp:3210-3254): the PREPARING leg above is now
// exact via handleCancelCast / interruptCurrentCast; the remaining legs are
// covered or intentionally absent.
//   - SPELL_STATE_DELAYED: Go has no DELAYED (missile in flight) state, so the
//     "interrupted if not delayed" term has nothing to match.
//   - m_autoRepeat = false: Go auto-repeat lives on the session
//     (autoRepeatSpell/autoRepeatTarget, cleared with SMSG_CANCEL_AUTO_REPEAT
//     in combat.go), not on a per-cast Spell object; there is no flag to clear
//     here.
//   - SPELL_STATE_CASTING (channeled): covered by interruptCurrentChannel,
//     which stops the timers, expires the channel aura on caster and target
//     (the RemoveOwnedAura(AURA_REMOVE_BY_CANCEL) mirror) and sends
//     SMSG_CHANNEL_UPDATE 0. The m_appliedMods.clear() term has no bridge: Go
//     has no per-cast Spell object to hold applied mods (standing gap noted in
//     spellmod.go). A CMSG_CANCEL_CAST naming a channeled spell id also
//     interrupts the channel in C++ (InterruptNonMeleeSpells always checks
//     CURRENT_CHANNELED_SPELL); Go only interrupts channels on
//     CMSG_CANCEL_CHANNELLING, which is the opcode the client actually sends.
//   - originalCaster RemoveDynObject/RemoveGameObject: Go has no gameobject
//     casters; every cast is a player session.
//   - the finish(false) tail: subsumed — activeCast is already nilled inline
//     and finishSpellCast early-returns on Cancelled, so no state lingers.

func (s *session) handleCancelChanneling(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	// Reference: cancel clears the running channel and its bar.
	s.interruptCurrentChannel()
	return true
}

func (s *session) handleCancelAura(payload []byte) bool {
	reader := protocol.NewReader(payload)
	spellID, err := reader.ReadU32()
	if err != nil {
		return false
	}
	s.removeAura(spellID)
	return true
}

// activeAura tracks an applied periodic or timed aura on a unit (player or creature).
type activeAura struct {
	SpellID                    uint32
	DispelType                 uint32
	Mechanic                   uint32
	AuraType                   uint32
	EffectMask                 uint8
	CasterGUID                 uint64
	TargetGUID                 uint64
	ChannelTargetGUID          uint64
	TargetKey                  creatureAuraKey
	ItemGUID                   uint64
	SchoolMask                 uint32
	MiscValue                  int32
	Amount                     uint32
	Amounts                    [3]int32
	BaseAmounts                [3]int32
	RecalculateMask            uint8
	CritChance                 float32
	ApplyResilience            bool
	DurationMs                 uint32
	PeriodMs                   uint32
	RemainingMs                uint32
	DurationUpdatedAt          time.Time
	Slot                       uint8
	Positive                   bool
	CasterLevel                uint8
	StackCount                 uint8
	SingleTarget               bool
	RemainingCharges           uint8
	StackAmount                uint32
	HideDuration               bool
	AuraInterruptFlags         uint32
	TriggerSpell               uint32
	DRGroup                    DiminishingGroup
	DamageTaken                uint32
	OwnerPetAura               bool
	OwnerPetAuraSourceSpell    uint32
	OwnerPetAuraSourceEffect   uint8
	OwnerPetAuraSourceDamage   int32
	OwnerPetAuraRemoveOnChange bool
	Timer                      *time.Timer
	TickTimer                  *time.Timer
	Stopped                    bool
}

func isHarmfulAura(auraType uint32) bool {
	switch auraType {
	case 3: // SPELL_AURA_PERIODIC_DAMAGE
		return true
	case 5: // SPELL_AURA_MOD_CONFUSE
		return true
	case 6: // SPELL_AURA_MOD_CHARM
		return true
	case 7: // SPELL_AURA_MOD_FEAR
		return true
	case 11: // SPELL_AURA_MOD_TAUNT
		return true
	case 12: // SPELL_AURA_MOD_STUN
		return true
	case 26: // SPELL_AURA_MOD_ROOT
		return true
	case 27: // SPELL_AURA_MOD_SILENCE
		return true
	case 33: // SPELL_AURA_MOD_DECREASE_SPEED
		return true
	case 53: // SPELL_AURA_PERIODIC_LEECH
		return true
	case 89: // SPELL_AURA_PERIODIC_DAMAGE_PERCENT
		return true
	default:
		return false
	}
}

func isHarmfulSpell(spell wotlk.Spell) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == 0 {
			continue
		}
		if eff.Effect == 1 || eff.Effect == 2 || eff.Effect == 87 || eff.Effect == 108 || eff.Effect == 17 {
			return true
		}
		if eff.Effect == 6 && isHarmfulAura(eff.Aura) {
			return true
		}
		if eff.Effect == 129 {
			return true
		}
		if eff.Effect == 27 && (eff.TriggerSpell != 0 || eff.Aura == 3 || eff.Aura == 89 || isAreaEnemyTargetType(eff.ImplicitTargetA) || isAreaEnemyTargetType(eff.ImplicitTargetB) || eff.ImplicitTargetA == 18 || eff.ImplicitTargetB == 18) {
			return true
		}
		if eff.ImplicitTargetA == 2 || eff.ImplicitTargetA == 6 || eff.ImplicitTargetA == 15 || eff.ImplicitTargetA == 16 || eff.ImplicitTargetA == 24 || eff.ImplicitTargetA == 54 || eff.ImplicitTargetA == 104 {
			return true
		}
	}
	return false
}

func magicSpellHitResult(casterLevel, victimLevel uint8, isPlayerVictim bool, bonusHit ...float64) uint8 {
	lchance := int32(11)
	if isPlayerVictim {
		lchance = 7
	}
	leveldif := int32(victimLevel) - int32(casterLevel)
	var modHitChance float64
	if leveldif < 3 {
		modHitChance = float64(96 - leveldif)
	} else {
		modHitChance = float64(94 - (leveldif-2)*lchance)
	}
	if len(bonusHit) > 0 {
		modHitChance += bonusHit[0]
	}
	if modHitChance < 1.0 {
		modHitChance = 1.0
	} else if modHitChance > 100.0 {
		modHitChance = 100.0
	}
	roll := rand.Float64() * 100.0
	if roll >= modHitChance {
		return protocol.SpellMissMiss
	}
	return protocol.SpellMissNone
}

// isBinarySpell checks whether a spell is binary (i.e. does not deal direct damage,
// but applies harmful magic debuffs or crowd control that can be fully resisted).
// Mirrors TrinityCore SpellInfo::IsBinary (SpellInfo.cpp:3200).
func isBinarySpell(spell wotlk.Spell) bool {
	// Evaluate on the stripped mask: C++ removes the physical bit at load when
	// magic schools are present (SpellMgr.cpp: CU_SCHOOLMASK_NORMAL_WITH_MAGIC).
	schoolMask := spell.SchoolMask
	if schoolMask&0x7E != 0 {
		schoolMask &^= 1
	}
	// Physical (1) and Holy (2) spells are never resisted by magic resistance
	if schoolMask == 0 || schoolMask&1 != 0 || schoolMask&2 != 0 {
		return false
	}
	if !isHarmfulSpell(spell) {
		return false
	}
	// Direct damage spells suffer partial resistance rather than binary full resist
	for _, eff := range spell.Effects {
		if eff.Effect == 2 || eff.Effect == 17 || eff.Effect == 31 || eff.Effect == 58 || eff.Effect == 87 {
			return false
		}
	}
	return true
}

// minResistanceForMask mirrors Unit::GetResistance(SpellSchoolMask)
// (Unit.cpp:13581): the minimum resistance across the schools in the mask.
// The physical bit is stripped when magic schools are present, mirroring the
// load-time SchoolMask normalization (SpellMgr.cpp:
// SPELL_ATTR0_CU_SCHOOLMASK_NORMAL_WITH_MAGIC).
func minResistanceForMask(resistances [7]uint32, schoolMask uint8) uint32 {
	if schoolMask&0x7E != 0 {
		schoolMask &^= 1
	}
	best := uint32(0)
	found := false
	for i := uint8(0); i < 7; i++ {
		if schoolMask&(1<<i) != 0 && (!found || resistances[i] < best) {
			best = resistances[i]
			found = true
		}
	}
	if !found {
		return ^uint32(0)
	}
	return best
}

// averageResistReduction mirrors Unit::CalculateAverageResistReduction
// (Unit.cpp:1777): template resistance reduced by spell penetration; holy
// template resistance and Chaos Bolt (warlock, icon 3178) template resistance
// are ignored; the level-based term (+5 per level the victim out-levels the
// caster, impenetrable) is skipped for binary spells.
func averageResistReduction(resistances [7]uint32, schoolMask uint8, casterPenetration uint32, casterLevel, victimLevel uint8, binary, chaosBolt bool) float64 {
	templateRes := minResistanceForMask(resistances, schoolMask)
	if schoolMask&2 != 0 || chaosBolt {
		templateRes = 0
	}
	effectiveRes := templateRes
	if casterPenetration >= effectiveRes {
		effectiveRes = 0
	} else {
		effectiveRes -= casterPenetration
	}

	res := float64(effectiveRes)
	if !binary && victimLevel > casterLevel {
		res += float64(victimLevel-casterLevel) * 5.0
	}

	const bossLevel = 83
	const bossResistanceConstant = 510.0
	resConstant := float64(victimLevel) * 5.0
	if victimLevel == bossLevel {
		resConstant = bossResistanceConstant
	}
	if res <= 0 {
		return 0
	}
	return res / (res + resConstant)
}

// checkBinarySpellResist rolls the full resist for a binary spell, with the
// average resist reduction as the resist chance. Mirrors the binary branch of
// WorldObject::MagicSpellHitResult (Object.cpp:2585-2592).
func checkBinarySpellResist(resistances [7]uint32, schoolMask uint8, casterPenetration uint32, casterLevel, victimLevel uint8, chaosBolt bool) bool {
	averageResist := averageResistReduction(resistances, schoolMask, casterPenetration, casterLevel, victimLevel, true, chaosBolt)
	if averageResist <= 0.0 {
		return false
	}
	if averageResist > 1.0 {
		averageResist = 1.0
	}

	return rand.Float64() < averageResist
}

// schoolMaskToResistanceIndex converts SpellSchoolMask to resistance index:
// 0: Physical (Armor), 1: Holy, 2: Fire, 3: Nature, 4: Frost, 5: Shadow, 6: Arcane.
func schoolMaskToResistanceIndex(schoolMask uint8) uint8 {
	switch {
	case schoolMask&1 != 0:
		return 0
	case schoolMask&2 != 0:
		return 1
	case schoolMask&4 != 0:
		return 2
	case schoolMask&8 != 0:
		return 3
	case schoolMask&16 != 0:
		return 4
	case schoolMask&32 != 0:
		return 5
	case schoolMask&64 != 0:
		return 6
	default:
		return 0
	}
}

// calcMagicSpellResistance mirrors Unit::CalcSpellResistedDamage
// (Unit.cpp:1704): the discrete 0%-100% partial-resist roll in 10% steps,
// truncated like the C++ uint32 cast, with a fall-through roll resolving to
// full resist (clamped by the caller like DamageInfo::ResistDamage,
// Unit.cpp:208). Holy can only be partially resisted by creatures (the
// level-based term; template holy resistance is ignored); non-magical
// schools are never resisted.
func calcMagicSpellResistance(damage uint32, schoolMask uint8, resistances [7]uint32, casterLevel, victimLevel uint8, victimIsCreature, chaosBolt bool, casterPenetration ...uint32) (resisted uint32, remainingDamage uint32) {
	if damage == 0 || schoolMask == 0 || schoolMask&0x7E == 0 {
		return 0, damage
	}
	if schoolMask&2 != 0 && !victimIsCreature {
		return 0, damage
	}

	pen := uint32(0)
	if len(casterPenetration) > 0 {
		pen = casterPenetration[0]
	}

	averageResist := averageResistReduction(resistances, schoolMask, pen, casterLevel, victimLevel, false, chaosBolt)
	if averageResist <= 0.0 {
		return 0, damage
	}

	var discreteProb [11]float64
	if averageResist <= 0.1 {
		discreteProb[0] = 1.0 - 7.5*averageResist
		discreteProb[1] = 5.0 * averageResist
		discreteProb[2] = 2.5 * averageResist
	} else {
		for i := 0; i < 11; i++ {
			p := 0.5 - 2.5*math.Abs(0.1*float64(i)-averageResist)
			if p > 0 {
				discreteProb[i] = p
			}
		}
	}

	roll := rand.Float64()
	probSum := 0.0
	step := 11
	for i := 0; i < 11; i++ {
		probSum += discreteProb[i]
		if roll < probSum {
			step = i
			break
		}
	}

	resisted = damage * uint32(step) / 10
	if resisted > damage {
		resisted = damage
	}
	remainingDamage = damage - resisted
	return resisted, remainingDamage
}

func (s *Server) clearCreatureAuras(key creatureAuraKey) {
	if s == nil || key.GUID == 0 {
		return
	}
	s.auraMu.Lock()
	defer s.auraMu.Unlock()
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok {
			for _, aura := range auras {
				if aura != nil {
					aura.Stopped = true
					if aura.Timer != nil {
						aura.Timer.Stop()
					}
					if aura.TickTimer != nil {
						aura.TickTimer.Stop()
					}
				}
			}
			delete(s.activeCreatureAuras, key)
		}
	}
	if s.creatureAuras != nil {
		delete(s.creatureAuras, key)
	}
}

func (s *session) clearActiveAuras() {
	if s == nil {
		return
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if s.activeAuras != nil {
		for _, aura := range s.activeAuras {
			if aura != nil {
				aura.Stopped = true
				if aura.Timer != nil {
					aura.Timer.Stop()
				}
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
				if aura.DRGroup != DiminishingNone {
					s.applyDiminishingAura(aura.DRGroup, false)
					aura.DRGroup = DiminishingNone
				}
			}
		}
		s.activeAuras = make(map[uint32]*activeAura)
	}
}

func (s *session) sendAuraUpdate(slot uint8, spellID uint32, remove, positive bool, maxDurationMs, durationMs uint32) {
	s.sendAuraUpdateWithStack(slot, spellID, remove, positive, maxDurationMs, durationMs, 1)
}

func (s *session) sendAuraUpdateWithStack(slot uint8, spellID uint32, remove, positive bool, maxDurationMs, durationMs uint32, stackCount uint8) {
	if !remove && s.server != nil && s.server.Data != nil {
		if spell, found, _ := s.server.Data.Spell(spellID); found {
			maxDurationMs, durationMs = auraWireDurations(spell, maxDurationMs, durationMs)
		}
	}
	effectMask := uint8(0x01)
	s.castMu.Lock()
	if aura := s.activeAuras[spellID]; aura != nil && aura.EffectMask != 0 {
		effectMask = aura.EffectMask
	}
	s.castMu.Unlock()
	level := uint8(1)
	if s.player != nil && s.player.Level > 0 {
		level = s.player.Level
	}
	pkt := protocol.BuildAuraUpdateWithStackEffect(s.playerGUID, s.playerGUID, slot, spellID, remove, positive, maxDurationMs, durationMs, level, stackCount, effectMask)
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), pkt, true)
}

func auraWireDurations(spell wotlk.Spell, maxDurationMs, durationMs uint32) (uint32, uint32) {
	if spell.AttributesEx5&spellAttr5HideDuration != 0 {
		return 0, 0
	}
	return maxDurationMs, durationMs
}

func spellEffectMask(spell wotlk.Spell, effect wotlk.SpellEffect) uint8 {
	for index, candidate := range spell.Effects {
		if candidate == effect && index < 8 {
			return uint8(1 << uint(index))
		}
	}
	return 0x01
}

func (s *session) applyAura(spellID uint32) {
	s.applyAuraWithDuration(spellID, 1800000)
}

func (s *session) applyAuraWithDuration(spellID uint32, durationMs uint32) {
	if s.auras == nil {
		s.auras = make(map[uint32]struct{})
	}
	if s.auraSlots == nil {
		s.auraSlots = make(map[uint32]uint8)
	}
	s.auras[spellID] = struct{}{}

	slot, ok := s.auraSlots[spellID]
	if !ok {
		used := make(map[uint8]bool)
		for _, sl := range s.auraSlots {
			used[sl] = true
		}
		var freeSlot uint8
		for sl := uint8(0); sl < 64; sl++ {
			if !used[sl] {
				freeSlot = sl
				break
			}
		}
		slot = freeSlot
		s.auraSlots[spellID] = slot
	}

	s.castMu.Lock()
	if s.activeAuras == nil {
		s.activeAuras = make(map[uint32]*activeAura)
	}
	if existing, exists := s.activeAuras[spellID]; exists && existing != nil {
		existing.Stopped = true
		if existing.Timer != nil {
			existing.Timer.Stop()
		}
		if existing.TickTimer != nil {
			existing.TickTimer.Stop()
		}
		// Aura::UnregisterSingleTarget (SpellAuras.cpp:1210): the replaced
		// aura leaves the caster's single-cast list.
		s.server.unregisterSingleCastAura(existing)
	}
	var auraInterruptFlags uint32
	var auraType uint32
	var dispelType uint32
	var mechanic uint32
	var miscValue int32
	var effectMask, recalculateMask uint8
	var amounts, baseAmounts [3]int32
	var effectAmount uint32
	stackCount := uint8(1)
	var stackAmount, procCharges uint32
	hideDuration := false
	if s.server != nil && s.server.Data != nil {
		if sp, found, _ := s.server.Data.Spell(spellID); found {
			auraInterruptFlags = sp.AuraInterruptFlags
			dispelType = sp.DispelType
			mechanic = sp.Mechanic
			stackAmount = sp.StackAmount
			hideDuration = sp.AttributesEx5&spellAttr5HideDuration != 0
			procCharges = sp.ProcCharges
			if sp.StackAmount == 0 && sp.ProcCharges > 0 {
				stackCount = uint8(sp.ProcCharges)
			}
			for index, effect := range sp.Effects {
				if effect.Effect == 0 || (index > 0 && auraType != 0 && effect.Aura != spellAuraMounted && !(auraType == 4 && effect.Aura == 4)) {
					continue
				}
				auraType = effect.Aura
				miscValue = effect.MiscValue
				if effect.Aura != 0 {
					effectMask |= 1 << uint(index)
					effectAmount = uint32(effect.BasePoints + 1)
					amounts[index] = effect.BasePoints + 1
					baseAmounts[index] = effect.BasePoints
					if auraEffectCanBeRecalculated(effect.Aura) {
						recalculateMask |= 1 << uint(index)
					}
				}
				if auraType == spellAuraMounted {
					break
				}
			}
			if mask, mountMisc, mountAmounts, mountBaseAmounts, mountedFlight := mountedFlightAuraEffects(sp); mountedFlight {
				effectMask, auraType, miscValue = mask, spellAuraMounted, mountMisc
				amounts, baseAmounts = mountAmounts, mountBaseAmounts
				recalculateMask = 0
				for index, effect := range sp.Effects {
					if mask&(1<<uint(index)) == 0 {
						continue
					}
					if auraEffectCanBeRecalculated(effect.Aura) {
						recalculateMask |= 1 << uint(index)
					}
					if effect.Aura == spellAuraMounted {
						effectAmount = uint32(effect.BasePoints + 1)
					}
				}
			}
		}
	}
	mounted := auraType == spellAuraMounted
	if mounted {
		s.clearOtherMountedAuras(spellID)
		durationMs = 0
		stackCount = 1
		procCharges = 0
	}
	positive := !isHarmfulAura(auraType)
	if spellID == 15007 {
		positive = false
	}
	if effectMask == 0 {
		effectMask = 0x01
	}
	aura := &activeAura{
		SpellID:            spellID,
		DispelType:         dispelType,
		Mechanic:           mechanic,
		AuraType:           auraType,
		EffectMask:         effectMask,
		CasterGUID:         s.playerGUID,
		TargetGUID:         s.playerGUID,
		MiscValue:          miscValue,
		Amount:             effectAmount,
		Amounts:            amounts,
		BaseAmounts:        baseAmounts,
		RecalculateMask:    recalculateMask,
		DurationMs:         durationMs,
		RemainingMs:        durationMs,
		DurationUpdatedAt:  time.Now(),
		SingleTarget:       isSingleTargetAuraSpellID(s.server.Data, spellID),
		Slot:               slot,
		Positive:           positive,
		AuraInterruptFlags: auraInterruptFlags,
		StackAmount:        stackAmount,
		HideDuration:       hideDuration,
		RemainingCharges:   uint8(procCharges),
	}
	if durationMs > 0 && durationMs < 18000000 {
		aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
			s.removeAura(spellID)
		})
	}
	s.activeAuras[spellID] = aura
	// Unit::_AddAura single-target dance (Unit.cpp:3397-3420), like the
	// applyAuraToTarget fresh-apply paths.
	var scPurge []singleCastEntry
	if s.server != nil && s.server.Data != nil {
		if sp, found, _ := s.server.Data.Spell(spellID); found && spellIsSingleTarget(sp) {
			scPurge = s.server.registerSingleCastAura(sp, aura)
		}
	}
	s.castMu.Unlock()
	// AuraEffect::ApplySpellMod on re-apply (SpellAuraEffects.cpp:755): the
	// replaced aura's modifiers leave before the new aura's register.
	s.dropSpellMods(spellID)
	s.addSpellMods(aura)
	for _, e := range scPurge {
		s.expireSingleCastEntry(e)
	}

	if s.activeAuraHasEffect(aura, spellAuraModParryPercent) {
		s.updatePlayerParryPercentage(s.player, s.player.Level)
	}
	if mounted {
		s.applyMountedDisplay(context.Background(), aura)
	}
	s.sendAuraUpdateWithStack(slot, spellID, false, positive, durationMs, durationMs, stackCount)
	s.sendPlayerUpdate()
	if auraType == 4 {
		s.addOwnerPetAuraEffects(context.Background(), spellID, effectMask)
	}
	if mounted {
		s.sendRuntimeMovementUpdates(spellAuraMounted)
	}
}

func (s *session) removeAura(spellID uint32) {
	wasMounted := false
	wasMovementControl := false
	wasConfused := false
	wasFleeing := false
	wasCharmed := false
	wasForcedReaction := false
	forcedReactionFaction := uint32(0)
	forcedReactionRank := uint32(0)
	wasTransform := false
	wasShapeshift := false
	wasStealth := false
	wasInvisibility := false
	wasTrackStealthed := false
	wasVisibilityAura := false
	wasMovementSpeedAura := false
	wasMountedFlight := false
	wasParryAura := false
	removedAuraType := uint32(0)
	removedFakeInebriation := uint32(0)
	removedEffectMask := uint8(0)
	s.castMu.Lock()
	if s.activeAuras != nil {
		if aura, ok := s.activeAuras[spellID]; ok && aura != nil {
			removedEffectMask = aura.EffectMask
			removedAuraType = aura.AuraType
			wasParryAura = s.activeAuraHasEffect(aura, spellAuraModParryPercent)
			wasMounted = aura.AuraType == spellAuraMounted
			wasMountedFlight = wasMounted && s.activeAuraHasEffect(aura, spellAuraMountedFlightSpeed)
			wasMovementControl = aura.AuraType == spellAuraStun || aura.AuraType == spellAuraRoot
			wasConfused = aura.AuraType == spellAuraConfuse
			wasFleeing = aura.AuraType == spellAuraFear
			wasCharmed = aura.AuraType == spellAuraCharm
			wasForcedReaction = aura.AuraType == 139
			if wasForcedReaction && aura.MiscValue >= 0 {
				forcedReactionFaction = uint32(aura.MiscValue)
				forcedReactionRank = aura.Amount
			}
			wasTransform = aura.AuraType == 56
			wasShapeshift = aura.AuraType == 36
			wasStealth = aura.AuraType == spellAuraStealth
			wasInvisibility = aura.AuraType == spellAuraInvisibility
			wasTrackStealthed = aura.AuraType == spellAuraTrackStealthed
			wasVisibilityAura = affectsPlayerVisibility(aura.AuraType)
			wasMovementSpeedAura = movementSpeedAura(aura.AuraType)
			if aura.AuraType == spellAuraFakeInebriation {
				removedFakeInebriation = aura.Amount
			}
			aura.Stopped = true
			if aura.Timer != nil {
				aura.Timer.Stop()
			}
			if aura.TickTimer != nil {
				aura.TickTimer.Stop()
			}
			if aura.DRGroup != DiminishingNone {
				s.applyDiminishingAura(aura.DRGroup, false)
				aura.DRGroup = DiminishingNone
			}
			delete(s.activeAuras, spellID)
			s.server.unregisterSingleCastAura(aura)
		}
	}
	s.castMu.Unlock()
	if removedEffectMask != 0 {
		s.dropSpellMods(spellID)
		s.removeOwnerPetAuraEffects(context.Background(), spellID, removedEffectMask)
	}
	s.removeOwnerPetAurasForSpell(context.Background(), spellID)

	if s.auras != nil {
		delete(s.auras, spellID)
	}
	if s.auraSlots != nil {
		if slot, ok := s.auraSlots[spellID]; ok {
			s.sendAuraUpdate(slot, 0, true, false, 0, 0)
			delete(s.auraSlots, spellID)
		}
	}
	if wasMounted && !s.hasAuraType(spellAuraMounted) && s.player != nil {
		s.player.MountDisplayID = 0
		s.sendPlayerMountUpdate()
		s.sendPlayerDismount()
	}
	if (wasTransform || wasShapeshift) && s.player != nil {
		s.refreshTransformDisplay(context.Background())
	}
	if wasStealth && s.player != nil && !s.hasAuraType(spellAuraStealth) {
		s.player.StandFlags &^= unitStandFlagCreep
		s.player.AuraVision &^= playerAuraVisionStealth
	}
	if wasInvisibility && s.player != nil && !s.hasAuraType(spellAuraInvisibility) {
		s.player.AuraVision &^= playerAuraVisionInvis
	}
	if wasTrackStealthed && s.player != nil && !s.hasAuraType(spellAuraTrackStealthed) {
		s.player.PlayerFieldBytes &^= playerFieldByteTrackStealthed
	}
	if wasMovementControl && !s.hasAuraType(spellAuraStun) && !s.hasAuraType(spellAuraRoot) {
		s.rooted = false
		if s.player != nil {
			s.player.UnitFlags &^= unitFlagStunned
			s.sendForcedMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_UNROOT))
		}
	}
	if wasConfused && !s.hasAuraType(spellAuraConfuse) && s.player != nil {
		s.player.UnitFlags &^= unitFlagConfused
	}
	if wasFleeing && !s.hasAuraType(spellAuraFear) && s.player != nil {
		s.player.UnitFlags &^= unitFlagFleeing
	}
	if wasCharmed && !s.hasAuraType(spellAuraCharm) {
		s.sendClientControl(s.playerGUID, true)
	}
	if wasForcedReaction {
		_ = s.sendForcedReactions()
		if forcedReactionFaction != 0 {
			rank := forcedReactionRank
			for _, reputation := range s.player.Reputations {
				if reputation.FactionID == forcedReactionFaction {
					if current := uint32(reputationRank(int64(reputation.Standing))); current > rank {
						rank = current
					}
					break
				}
			}
			if rank >= 4 {
				s.stopAttacksForFaction(context.Background(), forcedReactionFaction)
			}
		}
	}
	if removedFakeInebriation > 0 && s.player != nil {
		if removedFakeInebriation >= s.player.FakeInebriation {
			s.player.FakeInebriation = 0
		} else {
			s.player.FakeInebriation -= removedFakeInebriation
		}
	}
	if wasParryAura && s.player != nil {
		s.updatePlayerParryPercentage(s.player, s.player.Level)
	}
	s.sendPlayerUpdate()
	if wasVisibilityAura && s.server != nil {
		s.server.refreshPlayerVisibility()
	}
	if wasMovementSpeedAura {
		s.sendRuntimeMovementUpdates(removedAuraType)
	}
	if wasMountedFlight {
		s.sendRuntimeMovementUpdates(spellAuraMountedFlightSpeed)
	}
}

func (s *session) hasAura(spellID uint32) bool {
	if s.auras == nil {
		return false
	}
	_, ok := s.auras[spellID]
	return ok
}

// targetHasAura reports whether the unit named by targetGUID currently has aura
// auraSpell, for the target-side aura-spell requirement (SpellInfo::CheckTarget,
// SpellInfo.cpp:1769-1772). Player targets resolve to their session's aura set;
// creature targets consult the server creature aura maps.
func (s *session) targetHasAura(ctx context.Context, targetGUID uint64, auraSpell uint32) bool {
	if auraSpell == 0 || targetGUID == 0 {
		return false
	}
	if s.player != nil && targetGUID == s.playerGUID {
		return s.hasAura(auraSpell)
	}
	if s.server != nil {
		if targetSess := s.server.findSessionByGUID(targetGUID); targetSess != nil {
			return targetSess.hasAura(auraSpell)
		}
		target, ok := s.getCombatTarget(ctx, targetGUID)
		if ok {
			key := creatureAuraKeyForTarget(target)
			s.server.auraMu.Lock()
			defer s.server.auraMu.Unlock()
			if _, found := s.server.creatureAuras[key][auraSpell]; found {
				return true
			}
			_, found := s.server.activeCreatureAuras[key][auraSpell]
			return found
		}
	}
	return false
}

func (s *session) clearOtherMountedAuras(spellID uint32) {
	for _, aura := range s.loadedAuras() {
		if aura != nil && aura.AuraType == spellAuraMounted && aura.SpellID != spellID {
			s.removeAura(aura.SpellID)
		}
	}
}

func (s *session) applyMountedDisplay(ctx context.Context, aura *activeAura) {
	if s == nil || s.player == nil || aura == nil {
		return
	}
	entry := uint32(aura.MiscValue)
	if aura.SpellID == 62061 {
		if s.activeAuraHasEffect(aura, wotlk.MountedFlightSpeedAura) {
			entry = 24906
		} else {
			entry = 15665
		}
	}
	var displayID int64
	if entry > 0 && s.server != nil && s.server.WorldStore != nil && s.server.WorldStore.DB != nil {
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, "SELECT COALESCE(NULLIF(modelid1, 0), NULLIF(modelid2, 0), NULLIF(modelid3, 0), NULLIF(modelid4, 0), 0) FROM creature_template WHERE entry = ?", entry).Scan(&displayID); err == nil && displayID > 0 {
			s.player.MountDisplayID = uint32(displayID)
		}
	}
	s.sendPlayerMountUpdate()
}

func (s *session) refreshTransformDisplay(ctx context.Context) {
	if s == nil || s.player == nil {
		return
	}
	s.loadTransformDisplay(ctx, s.player)
}

// refreshAuraEffectBasepoints mirrors the per-effect half of the
// _TryStackingOrRefreshingExistingAura basepoint update (Unit.cpp:3360-3372):
// the re-cast's basepoints and computed amount replace this effect's slots.
func refreshAuraEffectBasepoints(existing *activeAura, spell wotlk.Spell, eff wotlk.SpellEffect, amount uint32) {
	// Aura::SetStackAmount (SpellAuras.cpp:1008) recalculates every effect's
	// amount from the new basepoints via AuraEffect::CalculateAmount, whose
	// final term is amount *= GetBase()->GetStackAmount()
	// (SpellAuraEffects.cpp:537) — unconditional, all aura types. The merge
	// path carries the new cast's amount directly (the CalculateAmount
	// pre-stack value), so the stack multiplier lands here; a zero
	// StackCount reads as one stack, the codebase's existing convention.
	stacks := int32(existing.StackCount)
	if stacks < 1 {
		stacks = 1
	}
	for index, candidate := range spell.Effects {
		if candidate != eff {
			continue
		}
		existing.BaseAmounts[index] = eff.BasePoints
		scaled := int32(amount) * stacks
		existing.Amounts[index] = scaled
		existing.Amount = uint32(scaled)
		break
	}
}

// castMerged tracks, for one cast, the target GUIDs whose existing aura
// already ran the re-apply merge; a nil map means the caller applies a
// single aura effect per spell and keeps the old always-merge behavior.

// spellFirstRank mirrors SpellInfo::GetFirstRankSpell (SpellInfo.cpp:3323)
// via the spell_ranks world table, cached by petAuraStackGroups. A spell
// with no rank row is its own first rank.
func (s *Server) spellFirstRank(spellID uint32) uint32 {
	if s == nil {
		return spellID
	}
	if first, ok := s.petAuraStackGroups().firstRank[spellID]; ok {
		return first
	}
	return spellID
}

// spellAuraScaleMask mirrors the "Fill aura scaling information" block in
// Spell::prepare (Spell.cpp:3040-3056). It returns the bit mask of effect
// indexes whose auras scale for low-level targets, or 0 when scaling does
// not apply. The caster is always player-controlled here (the caster is a
// player session), so the remaining gates are: the spell is non-passive,
// carries a SpellLevel, is not channeled, and the cast is not triggered —
// every Go triggered cast rides TRIGGERED_FULL_MASK (0x0007FFFF), which
// includes TRIGGERED_IGNORE_AURA_SCALING (0x10, SpellDefines.h:138), while
// finishSpellCast only serves client-initiated casts. A bit is set for each
// positive SPELL_EFFECT_APPLY_AURA effect; the whole mask is dropped when
// the cast's basepoints were taken from anywhere but the spell's own DBC
// row (m_spellValue->EffectBasePoints[i] != m_spellInfo->Effects[i].BasePoints,
// Spell.cpp:3051-3055).
func spellAuraScaleMask(spell, pristine wotlk.Spell, pristineOK bool) uint8 {
	if spell.Attributes&spellAttributePassive != 0 {
		return 0
	}
	if spell.SpellLevel == 0 {
		return 0
	}
	if isChanneledSpell(spell) {
		return 0
	}
	var mask uint8
	for i := range spell.Effects {
		if spell.Effects[i].Effect != spellEffectApplyAura {
			continue
		}
		if !spell.IsPositiveEffect(i) {
			continue
		}
		mask |= 1 << uint(i)
		if pristineOK && spell.Effects[i].BasePoints != pristine.Effects[i].BasePoints {
			return 0
		}
	}
	return mask
}

// auraScaleTargetLevel resolves a hit target's level for the aura-scaling
// min-level check (Spell::AddUnitTarget, Spell.cpp:2123/2143); online
// players and tracked creatures both resolve through getCombatTarget.
func (s *session) auraScaleTargetLevel(ctx context.Context, targetGUID uint64) (uint8, bool) {
	if tgt, ok := s.getCombatTarget(ctx, targetGUID); ok && tgt.Level > 0 {
		return tgt.Level, true
	}
	return 0, false
}

// spellAuraRankForLevel mirrors SpellInfo::GetAuraRankForLevel
// (SpellInfo.cpp:3283-3319): it walks the spell_ranks chain downward from
// the cast spell's rank and returns the highest rank whose SpellLevel fits
// targetLevel+10. The cast spell itself is returned when no downrank
// applies — the needRankSelection gate (no positive APPLY_AURA-family
// effect), the passive/attribute gates (SPELL_ATTR0_NEGATIVE_1,
// SPELL_ATTR2_UNK3 "Ignore aura scaling", SPELL_ATTR3_DRAIN_SOUL), a
// missing rank row, or an effect-type mismatch between the ranks (the
// PreprocessSpellHit sanity leg, Spell.cpp:2785-2794). ok is false only when
// the spell data itself is unavailable.
func (s *Server) spellAuraRankForLevel(spellID uint32, targetLevel uint8) (wotlk.Spell, bool) {
	if s == nil || s.Data == nil {
		return wotlk.Spell{}, false
	}
	cast, found, err := s.Data.Spell(spellID)
	if err != nil || !found {
		return wotlk.Spell{}, false
	}
	needRank := false
	for i := range cast.Effects {
		eff := cast.Effects[i].Effect
		if cast.IsPositiveEffect(i) && (eff == spellEffectApplyAura || eff == spellEffectApplyAreaAuraParty || eff == spellEffectApplyAreaAuraRaid) {
			needRank = true
			break
		}
	}
	if !needRank || cast.Attributes&spellAttributePassive != 0 ||
		cast.Attributes&spellAttr0Negative1 != 0 ||
		cast.AttributesEx1&spellAttr2Unk3 != 0 ||
		cast.AttributesEx3&spellAttr3DrainSoul != 0 {
		return cast, true
	}
	groups := s.petAuraStackGroups()
	first := groups.firstRank[spellID]
	if first == 0 {
		first = spellID
	}
	var rank uint32
	for r := uint32(1); r <= 64; r++ {
		id, ok := groups.rankSpell[uint64(first)<<32|uint64(r)]
		if !ok {
			break
		}
		if id == spellID {
			rank = r
			break
		}
	}
	if rank == 0 {
		// No rank row: C++ tests the cast spell itself once, then gives up
		// (nullptr), which the caller treats as the cast spell.
		return cast, true
	}
	for r := rank; r >= 1; r-- {
		id, ok := groups.rankSpell[uint64(first)<<32|uint64(r)]
		if !ok {
			continue
		}
		down, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		if uint32(targetLevel)+10 < down.SpellLevel {
			continue
		}
		for i := range cast.Effects {
			if cast.Effects[i].Effect != down.Effects[i].Effect {
				return cast, true
			}
		}
		return down, true
	}
	return cast, true
}

// applyAuraScaling mirrors the AddUnitTarget ScaleAura arm
// (Spell.cpp:2115-2150) and the SelectTargets removal leg
// (Spell.cpp:807-830). For every hit target other than the caster whose
// effect mask is exactly the aura-scale mask, the target passes the min
// level check when targetLevel+10 reaches the first rank's SpellLevel;
// failing targets are dropped from the hit list, and when every target is
// dropped the cast fails with SPELL_FAILED_LOWLEVEL (Spell.cpp:827-830).
// Targets whose level cannot be resolved are kept unscaled. Passing
// lower-level targets are recorded in the returned map with their
// downranked spell row — the PreprocessSpellHit scaleAura leg
// (Spell.cpp:2780-2795) that applies the aura with the rank-appropriate
// SpellInfo and basepoints.
func (s *session) applyAuraScaling(ctx context.Context, spellID uint32, spell wotlk.Spell, mask uint8, hitTargets []uint64) ([]uint64, map[uint64]wotlk.Spell, bool) {
	var targetMask uint8
	for i := range spell.Effects {
		if spell.Effects[i].Effect != 0 {
			targetMask |= 1 << uint(i)
		}
	}
	var firstLevel uint32
	if s.server != nil {
		if fr, found, _ := s.server.Data.Spell(s.server.spellFirstRank(spellID)); found {
			firstLevel = fr.SpellLevel
		}
	}
	downranks := make(map[uint64]wotlk.Spell)
	kept := make([]uint64, 0, len(hitTargets))
	for _, guid := range hitTargets {
		if guid == 0 || guid == s.playerGUID || targetMask != mask {
			kept = append(kept, guid)
			continue
		}
		level, ok := s.auraScaleTargetLevel(ctx, guid)
		if !ok || uint32(level)+10 < firstLevel {
			continue
		}
		if s.server != nil {
			if ds, ok := s.server.spellAuraRankForLevel(spellID, level); ok && ds.ID != spellID {
				downranks[guid] = ds
			}
		}
		kept = append(kept, guid)
	}
	if len(hitTargets) > 0 && len(kept) == 0 {
		return nil, nil, true
	}
	return kept, downranks, false
}

// auraEffectParams computes the duration, period and base amount for one
// APPLY_AURA-family effect application from the given spell row and effect;
// the downranked-aura leg (Spell::PreprocessSpellHit scaleAura,
// Spell.cpp:2780-2795, via hitInfo.AuraSpellInfo) evaluates these from the
// rank-appropriate row instead of the cast spell's.
func (s *session) auraEffectParams(spell wotlk.Spell, eff wotlk.SpellEffect) (durationMs, periodMs, amount uint32) {
	if spell.DurationIndex > 0 && s.server != nil && s.server.Data != nil && s.player != nil {
		if val, ok, err := s.server.Data.SpellDuration(spell.DurationIndex, uint32(s.player.Level)); err == nil && ok && val > 0 {
			durationMs = uint32(val)
		}
	}
	if durationMs == 0 && eff.AuraPeriod > 0 {
		durationMs = eff.AuraPeriod * 5
	}
	periodMs = eff.AuraPeriod
	if periodMs == 0 && (eff.Aura == 3 || eff.Aura == 8 || eff.Aura == 23 || eff.Aura == 24 || eff.Aura == 89) {
		periodMs = 3000
	}
	amount = uint32(eff.BasePoints + 1)
	if amount <= 1 && isAreaEnemySpell(spell) {
		for _, areaEffect := range spell.Effects {
			if areaEffect.Effect == 27 && areaEffect.BasePoints >= 0 {
				amount = uint32(areaEffect.BasePoints + 1)
				break
			}
		}
	}
	if amount == 0 {
		if s.player != nil {
			if eff.Aura == 3 || eff.Aura == 23 || eff.Aura == 89 {
				amount = uint32(10 + int(s.player.Level)*2)
			} else if eff.Aura == 8 || eff.Aura == 20 {
				amount = uint32(15 + int(s.player.Level)*3)
			}
		}
	}
	// Spell power bonus for periodic effects and absorption shields (TrinityCore Unit::SpellDamageBonusDone / SpellHealingBonusDone)
	if s.player != nil && s.player.SpellPower > 0 {
		if periodMs > 0 && (eff.Aura == 3 || eff.Aura == 23 || eff.Aura == 89 || eff.Aura == 8 || eff.Aura == 20) {
			tickBonus := uint32(math.Round(float64(s.player.SpellPower) * (float64(periodMs) / 15000.0)))
			amount += tickBonus
		} else if eff.Aura == SpellAuraSchoolAbsorb || eff.Aura == SpellAuraManaShield || eff.Aura == SpellAuraMagicAbsorb {
			shieldBonus := uint32(math.Round(float64(s.player.SpellPower) * 0.8068))
			amount += shieldBonus
		}
	}
	return durationMs, periodMs, amount
}

func auraTriggersSpell(spell wotlk.Spell, target uint32) bool {
	for _, eff := range spell.Effects {
		if eff.TriggerSpell == target {
			return true
		}
	}
	return false
}

// rankPurgeTarget is one aura the rank-chain no-stack purge selected for
// removal; the caller removes them after unlocking.
type rankPurgeTarget struct {
	spellID uint32
	slot    uint8
}

// Periodic aura types exempted from the different-caster rank-chain purge
// (Aura::CanStackWith, SpellAuras.cpp:1955-1976).
const (
	spellAuraPeriodicDamage                = 3   // SPELL_AURA_PERIODIC_DAMAGE (SpellAuraDefines.h:83)
	spellAuraPeriodicHeal                  = 8   // SPELL_AURA_PERIODIC_HEAL (SpellAuraDefines.h:88)
	spellAuraObsModHealth                  = 20  // SPELL_AURA_OBS_MOD_HEALTH (SpellAuraDefines.h:100)
	spellAuraObsModPower                   = 21  // SPELL_AURA_OBS_MOD_POWER (SpellAuraDefines.h:101)
	spellAuraPeriodicTriggerSpell          = 23  // SPELL_AURA_PERIODIC_TRIGGER_SPELL (SpellAuraDefines.h:103)
	spellAuraPeriodicEnergize              = 24  // SPELL_AURA_PERIODIC_ENERGIZE (SpellAuraDefines.h:104)
	spellAuraPeriodicLeech                 = 53  // SPELL_AURA_PERIODIC_LEECH (SpellAuraDefines.h:133)
	spellAuraPeriodicManaLeech             = 64  // SPELL_AURA_PERIODIC_MANA_LEECH (SpellAuraDefines.h:144)
	spellAuraPowerBurn                     = 162 // SPELL_AURA_POWER_BURN (SpellAuraDefines.h:242)
	spellAuraPeriodicDummy                 = 226 // SPELL_AURA_PERIODIC_DUMMY (SpellAuraDefines.h:306)
	spellAuraPeriodicTriggerSpellWithValue = 227 // SPELL_AURA_PERIODIC_TRIGGER_SPELL_WITH_VALUE (SpellAuraDefines.h:307)
)

// rankChainNoStackPurge mirrors the rank-chain term of
// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640-3671) via
// Aura::CanStackWith (SpellAuras.cpp:1880-2007): a fresh aura application
// removes the target's existing auras that share its spell rank chain under
// a different spell ID. Both caster cases are modeled: same-caster pairs
// always purge (SpellAuras.cpp:2004); different-caster pairs purge unless
// C++ lets them stack — the channeled exemption, the
// SPELL_ATTR3_STACK_FOR_DIFF_CASTERS read, and the periodic-aura-type
// exemption (SpellAuras.cpp:1943-1976) — so two players' Corruption ranks on
// one mob coexist while a single player's re-cast at a different rank
// replaces the old one.
// Honored in both directions: the trigger-spell mutual exclusion
// (SpellAuras.cpp:1901-1906), the spell-family gate (SpellAuras.cpp:1934),
// and the enchant-proc item edge (SpellAuras.cpp:1998-2000, degrade-open
// without the CU attr). Passive new spells skip the purge (the
// IsPassiveStackableWithRanks early-out shape, Unit.cpp:3643; Go holds no
// passive aura instances). IsMultiSlotAura (SpellAuras.cpp:1148) and
// CONTROL_VEHICLE are vacuous in Go. Spell-group stack rules are handled by
// the companion spellGroupNoStackPurge (rules EXCLUSIVE and
// EXCLUSIVE_FROM_SAME_CASTER) and exclusiveHighestVerdict (rule
// EXCLUSIVE_HIGHEST, both directions).
// Returns the auras to remove; the caller removes them after unlocking.
func (s *Server) rankChainNoStackPurge(newSpell wotlk.Spell, newCasterGUID, newItemGUID uint64, existing map[uint32]*activeAura) []rankPurgeTarget {
	if s == nil || s.Data == nil {
		return nil
	}
	if newSpell.Attributes&spellAttributePassive != 0 {
		return nil
	}
	newFirst := s.spellFirstRank(newSpell.ID)
	var purge []rankPurgeTarget
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		if s.spellFirstRank(id) != newFirst {
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
			continue
		}
		if exSpell.SpellFamilyName != newSpell.SpellFamilyName {
			continue
		}
		if aura.CasterGUID != newCasterGUID {
			if newSpell.AttributesEx3&spellAttr3StackForDiffCasters != 0 {
				continue
			}
			if isChanneledSpell(exSpell) {
				continue
			}
			if rankChainPeriodicStacksForDiffCasters(newSpell, exSpell) {
				continue
			}
		}
		if newItemGUID != 0 && aura.ItemGUID != 0 && newItemGUID != aura.ItemGUID {
			continue
		}
		purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
	}
	return purge
}

// spellIsSingleTarget mirrors SpellInfo::IsSingleTarget
// (SpellInfo.cpp:1380-1395): the SPELL_ATTR5_SINGLE_TARGET_SPELL flag, or
// the JUDGEMENT spell-specific. The nil firstRank resolver is safe — the
// classifier guards it, and the judgement branch is ID-based.
func spellIsSingleTarget(spell wotlk.Spell) bool {
	if spell.AttributesEx5&spellAttr5SingleTarget != 0 {
		return true
	}
	return spellSpecific(spell, nil) == spellSpecificJudgement
}

// isSingleTargetWith mirrors Aura::IsSingleTargetWith
// (SpellAuras.cpp:1187-1208): same rank chain, or both spells sharing the
// JUDGEMENT / MAGE_POLYMORPH spell-specific.
func (s *Server) isSingleTargetWith(newSpell, exSpell wotlk.Spell) bool {
	if s.spellFirstRank(newSpell.ID) == s.spellFirstRank(exSpell.ID) {
		return true
	}
	newSpec := spellSpecific(newSpell, s.spellFirstRank)
	switch newSpec {
	case spellSpecificJudgement, spellSpecificMagePolymorph:
		return spellSpecific(exSpell, s.spellFirstRank) == newSpec
	}
	return false
}

// singleTargetNoStackPurge mirrors the single-target registration dance in
// Unit::_AddAura (Unit.cpp:3397-3420) with Aura::IsSingleTargetWith
// (SpellAuras.cpp:1187-1208): a fresh single-target aura (SpellInfo::
// IsSingleTarget, SpellInfo.cpp:1380-1395) removes the caster's other
// single-target auras that are single-target with it — same-rank-chain
// pairs, or a second judgement / polymorph from the same caster. The
// trigger-spell mutual exclusion the _RemoveNoStackAurasDueToAura purges
// honor does not exist in the C++ dance, so it is not applied here. C++
// dances the caster's cross-target single-cast list; Go has no such list,
// so this covers the same-target case (re-judging / re-sheeping the same
// unit) and the cross-target remainder stays open. Returns the auras to
// remove; the caller removes them after unlocking.
func (s *Server) singleTargetNoStackPurge(newSpell wotlk.Spell, newCasterGUID uint64, existing map[uint32]*activeAura) []rankPurgeTarget {
	if s == nil || s.Data == nil || !spellIsSingleTarget(newSpell) {
		return nil
	}
	var purge []rankPurgeTarget
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		if aura.CasterGUID != newCasterGUID {
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found || !spellIsSingleTarget(exSpell) {
			continue
		}
		if !s.isSingleTargetWith(newSpell, exSpell) {
			continue
		}
		purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
	}
	return purge
}

// singleCastEntry is one single-target aura registered on its caster,
// mirroring an element of Unit::m_scAuras (Unit.h:1287-1288). The aura
// pointer identifies the exact instance: removals verify it before expiring,
// so a re-created aura with the same spell ID is never touched.
type singleCastEntry struct {
	aura *activeAura
}

// sameSingleCastTarget reports whether two registry entries live on the same
// target: creature entries compare the full aura key, player entries the
// target GUID.
func sameSingleCastTarget(a, b *activeAura) bool {
	if a.TargetKey.GUID != 0 || b.TargetKey.GUID != 0 {
		return a.TargetKey == b.TargetKey
	}
	return a.TargetGUID == b.TargetGUID
}

// singleCastDanceLocked implements the single-target registration dance from
// Unit::_AddAura (Unit.cpp:3397-3420): when register is non-nil the fresh
// single-target aura is recorded on its caster's single-cast list
// (Unit::m_scAuras), replacing any entry for the same target+spell. Either
// way it returns the caster's other registered auras that are single-target
// with the new spell (Aura::IsSingleTargetWith, SpellAuras.cpp:1187-1208) on
// a different target — the cross-target dance. Same-target entries are left
// alone: that case is owned by singleTargetNoStackPurge. The caller must hold
// no aura locks; it removes the returned entries after unlocking via
// expireSingleCastEntry. Caller holds s.singleCastMu.
func (s *Server) singleCastDanceLocked(spell wotlk.Spell, register *activeAura, casterGUID uint64, exclude *activeAura) []singleCastEntry {
	var kept []singleCastEntry
	var purge []singleCastEntry
	for _, e := range s.singleCastAuras[casterGUID] {
		if e.aura == nil || e.aura == register || e.aura == exclude {
			continue
		}
		if register != nil && sameSingleCastTarget(e.aura, register) {
			// Same target: the new registration replaces a same-spell
			// entry; a different-spell entry stays registered and is
			// removed by the same-target purge (unregister cleans up).
			if e.aura.SpellID == register.SpellID {
				continue
			}
			kept = append(kept, e)
			continue
		}
		if s.Data == nil {
			kept = append(kept, e)
			continue
		}
		exSpell, found, err := s.Data.Spell(e.aura.SpellID)
		if err != nil || !found || !s.isSingleTargetWith(spell, exSpell) {
			kept = append(kept, e)
			continue
		}
		purge = append(purge, e)
	}
	if register != nil {
		kept = append(kept, singleCastEntry{aura: register})
	}
	if len(kept) == 0 {
		delete(s.singleCastAuras, casterGUID)
	} else {
		s.singleCastAuras[casterGUID] = kept
	}
	return purge
}

// registerSingleCastAura mirrors the registration half of Unit::_AddAura's
// dance (Unit.cpp:3397-3420): a fresh single-target aura
// (SpellInfo::IsSingleTarget, SpellInfo.cpp:1380-1395) is recorded on its
// caster's single-cast list, and the caster's other single-target-with auras
// on other targets are returned for removal. C++ gates on a non-null caster;
// a zero caster GUID degrades to no registration.
func (s *Server) registerSingleCastAura(spell wotlk.Spell, aura *activeAura) []singleCastEntry {
	if s == nil || aura == nil || aura.CasterGUID == 0 {
		return nil
	}
	s.singleCastMu.Lock()
	defer s.singleCastMu.Unlock()
	if s.singleCastAuras == nil {
		s.singleCastAuras = make(map[uint64][]singleCastEntry)
	}
	return s.singleCastDanceLocked(spell, aura, aura.CasterGUID, nil)
}

// crossTargetSingleCastPurge runs only the purge half of the dance for a
// caster without registering anything, excluding one aura instance. Used by
// the spell-steal create path (below).
func (s *Server) crossTargetSingleCastPurge(casterGUID uint64, spell wotlk.Spell, exclude *activeAura) []singleCastEntry {
	if s == nil || casterGUID == 0 {
		return nil
	}
	s.singleCastMu.Lock()
	defer s.singleCastMu.Unlock()
	return s.singleCastDanceLocked(spell, nil, casterGUID, exclude)
}

// unregisterSingleCastAura mirrors Aura::UnregisterSingleTarget
// (SpellAuras.cpp:1210-1216): a removed aura leaves its caster's single-cast
// list. No-op when the aura was never registered (DB-loaded auras,
// steal-created auras, non-single-target spells).
func (s *Server) unregisterSingleCastAura(aura *activeAura) {
	if s == nil || aura == nil || aura.CasterGUID == 0 {
		return
	}
	s.singleCastMu.Lock()
	defer s.singleCastMu.Unlock()
	entries := s.singleCastAuras[aura.CasterGUID]
	for i, e := range entries {
		if e.aura == aura {
			entries[i] = entries[len(entries)-1]
			entries = entries[:len(entries)-1]
			break
		}
	}
	if len(entries) == 0 {
		delete(s.singleCastAuras, aura.CasterGUID)
	} else {
		s.singleCastAuras[aura.CasterGUID] = entries
	}
}

// expireSingleCastEntry removes a cross-target dance purge entry, but only
// when the registered aura instance is still the live one — mirroring the
// (*itr) != aura pointer check in Unit::_AddAura's dance loop
// (Unit.cpp:3409-3419). A re-created aura with the same spell ID is a
// different pointer and is never touched.
func (s *session) expireSingleCastEntry(e singleCastEntry) {
	if s == nil || s.server == nil || e.aura == nil {
		return
	}
	aura := e.aura
	if aura.TargetKey.GUID != 0 {
		key := aura.TargetKey
		s.server.auraMu.Lock()
		cur := s.server.activeCreatureAuras[key][aura.SpellID]
		s.server.auraMu.Unlock()
		if cur == aura {
			s.expireCreatureAura(key, aura.SpellID, aura.Slot)
		}
		return
	}
	if aura.TargetGUID == 0 {
		return
	}
	ts := s.server.findSessionByGUID(aura.TargetGUID)
	if ts == nil {
		return
	}
	ts.castMu.Lock()
	cur := ts.activeAuras[aura.SpellID]
	ts.castMu.Unlock()
	if cur == aura {
		ts.expirePlayerAura(aura.SpellID)
	}
}

// rankChainPeriodicStacksForDiffCasters mirrors the periodic-aura exemption
// in Aura::CanStackWith (SpellAuras.cpp:1955-1976): DOT/HOT-style auras of
// one rank chain from different casters stack. Effects are index-aligned
// exactly like C++ (the area gate reads Effects[i] on both spells); a
// periodic type targeting an area on either side (Replenishment-style)
// keeps the purge. isAreaAuraTarget was verified index-identical to
// SpellImplicitTargetInfo::IsArea from the C++ implicit-target table
// (SpellInfo.cpp:218-341). PERIODIC_DAMAGE_PERCENT (89) is deliberately
// absent — C++ lists only PERIODIC_DAMAGE (3).
func rankChainPeriodicStacksForDiffCasters(newSpell, exSpell wotlk.Spell) bool {
	for i, eff := range newSpell.Effects {
		switch eff.Aura {
		case spellAuraPeriodicDamage, spellAuraPeriodicDummy, spellAuraPeriodicHeal,
			spellAuraPeriodicTriggerSpell, spellAuraPeriodicEnergize,
			spellAuraPeriodicManaLeech, spellAuraPeriodicLeech,
			spellAuraPowerBurn, spellAuraObsModPower, spellAuraObsModHealth,
			spellAuraPeriodicTriggerSpellWithValue:
		default:
			continue
		}
		if isAreaAuraTarget(eff.ImplicitTargetA) || isAreaAuraTarget(eff.ImplicitTargetB) {
			continue
		}
		if i < len(exSpell.Effects) {
			ex := exSpell.Effects[i]
			if isAreaAuraTarget(ex.ImplicitTargetA) || isAreaAuraTarget(ex.ImplicitTargetB) {
				continue
			}
		}
		return true
	}
	return false
}

// Spell group stack rules (SpellMgr.h:326-334).
const (
	spellGroupStackRuleDefault             = 0 // SPELL_GROUP_STACK_RULE_DEFAULT
	spellGroupStackRuleExclusive           = 1 // SPELL_GROUP_STACK_RULE_EXCLUSIVE
	spellGroupStackRuleExclusiveSameCaster = 2 // SPELL_GROUP_STACK_RULE_EXCLUSIVE_FROM_SAME_CASTER
	spellGroupStackRuleExclusiveSameEffect = 3 // SPELL_GROUP_STACK_RULE_EXCLUSIVE_SAME_EFFECT
	spellGroupStackRuleExclusiveHighest    = 4 // SPELL_GROUP_STACK_RULE_EXCLUSIVE_HIGHEST
)

// spellGroupStackRule mirrors SpellMgr::CheckSpellGroupStackRules
// (SpellMgr.cpp:438-488): group membership is read by first-rank spell ID
// (the petAuraStackCache spellGroups map is already expanded and
// first-rank-keyed), groups that overlap on both spells only through a
// shared nested subgroup are excluded, and the lowest group ID carrying a
// non-default rule wins.
func (s *Server) spellGroupStackRule(first1, first2 uint32) uint8 {
	cache := s.petAuraStackGroups()
	g1, ok1 := cache.spellGroups[first1]
	g2, ok2 := cache.spellGroups[first2]
	if !ok1 || !ok2 {
		return spellGroupStackRuleDefault
	}
	var common []uint32
	for g := range g1 {
		if _, ok := g2[g]; !ok {
			continue
		}
		excluded := false
		for _, sub := range cache.groupSubgroups[g] {
			_, in1 := g1[sub]
			_, in2 := g2[sub]
			if in1 && in2 {
				excluded = true
				break
			}
		}
		if !excluded {
			common = append(common, g)
		}
	}
	sort.Slice(common, func(i, j int) bool { return common[i] < common[j] })
	for _, g := range common {
		if rule := cache.groupRules[g]; rule != spellGroupStackRuleDefault {
			return rule
		}
	}
	return spellGroupStackRuleDefault
}

// spellGroupNoStackPurge mirrors the CheckSpellGroupStackRules terms of
// Aura::CanStackWith (SpellAuras.cpp:1924-1942) inside
// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640-3671): a fresh aura
// purges existing auras whose first-rank spell shares an EXCLUSIVE group
// with the new spell, or an EXCLUSIVE_FROM_SAME_CASTER group when the
// caster matches. The trigger-spell mutual exclusion (SpellAuras.cpp:
// 1901-1906) is honored — a triggered/triggering pair stacks, so it never
// purges. Rule EXCLUSIVE_SAME_EFFECT falls through to break in C++ (no
// purge); rule EXCLUSIVE_HIGHEST is handled by the companion
// exclusiveHighestVerdict (the IsHighestExclusiveAura port, Unit.cpp:13991).
// Returns the auras to remove; the caller removes them after unlocking.
func (s *Server) spellGroupNoStackPurge(newSpell wotlk.Spell, newCasterGUID uint64, existing map[uint32]*activeAura) []rankPurgeTarget {
	if s == nil || s.Data == nil {
		return nil
	}
	if newSpell.Attributes&spellAttributePassive != 0 {
		return nil
	}
	newFirst := s.spellFirstRank(newSpell.ID)
	var purge []rankPurgeTarget
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		switch s.spellGroupStackRule(newFirst, s.spellFirstRank(id)) {
		case spellGroupStackRuleExclusive:
		case spellGroupStackRuleExclusiveSameCaster:
			if aura.CasterGUID != newCasterGUID {
				continue
			}
		default:
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
			continue
		}
		purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
	}
	return purge
}

// SpellSpecificType values (SpellInfo.h:149-174) for the
// SpellInfo::GetSpellSpecific classifier (_LoadSpellSpecific,
// SpellInfo.cpp:2041-2235).
const (
	spellSpecificNormal              = 0
	spellSpecificSeal                = 1
	spellSpecificAura                = 3
	spellSpecificSting               = 4
	spellSpecificCurse               = 5
	spellSpecificAspect              = 6
	spellSpecificTracker             = 7
	spellSpecificWarlockArmor        = 8
	spellSpecificMageArmor           = 9
	spellSpecificElementalShield     = 10
	spellSpecificMagePolymorph       = 11
	spellSpecificJudgement           = 13
	spellSpecificWarlockCorruption   = 17
	spellSpecificFood                = 19
	spellSpecificDrink               = 20
	spellSpecificFoodAndDrink        = 21
	spellSpecificPresence            = 22
	spellSpecificCharm               = 23
	spellSpecificScroll              = 24
	spellSpecificMageArcaneBrillance = 25
	spellSpecificWarriorEnrage       = 26
	spellSpecificPriestDivineSpirit  = 27
	spellSpecificHand                = 28
)

// Spell family names and aura/effect IDs consumed by the SpellSpecific
// classifier (SharedDefines.h:3581-3596, SpellAuraDefines.h, SpellDefines.h:65).
const (
	spellFamilyGeneric     = 0
	spellFamilyMage        = 3
	spellFamilyPriest      = 6
	spellFamilyHunter      = 9
	spellFamilyPaladin     = 10
	spellFamilyShaman      = 11
	spellFamilyDeathKnight = 15

	// SpellFamilyFlags[0] bit marking mage conjure food/water/refreshment
	// spells (the Spell.cpp:6892 SpellFamilyFlags[0] & 0x40000000 arm of the
	// CREATE_ITEM check).
	spellFamilyFlagConjureRefreshment = 0x40000000

	spellAuraModPossess     = 2
	spellAuraTrackCreatures = 44
	spellAuraTrackResources = 45
	spellAuraModRegen       = 84
	spellAuraModPowerRegen  = 85
	spellAuraModPossessPet  = 128
	spellAuraAoeCharm       = 177

	spellEffectApplyAura           = 6
	spellEffectPersistentAreaAura  = 27
	spellEffectApplyAreaAuraParty  = 35
	spellEffectApplyAreaAuraRaid   = 65
	spellEffectApplyAreaAuraPet    = 119
	spellEffectApplyAreaAuraFriend = 128
	spellEffectApplyAreaAuraEnemy  = 129
	spellEffectApplyAreaAuraOwner  = 143
)

// spellEffectIsAuraEffect mirrors SpellEffectInfo::IsAura (SpellInfo.cpp:370-373)
// via IsUnitOwnedAuraEffect (SpellAuras.cpp:273): an APPLY_AURA-family effect
// carrying a real aura type.
func spellEffectIsAuraEffect(eff wotlk.SpellEffect) bool {
	if eff.Aura == 0 {
		return false
	}
	switch eff.Effect {
	case spellEffectApplyAura, spellEffectPersistentAreaAura,
		spellEffectApplyAreaAuraParty, spellEffectApplyAreaAuraRaid,
		spellEffectApplyAreaAuraPet, spellEffectApplyAreaAuraFriend,
		spellEffectApplyAreaAuraEnemy, spellEffectApplyAreaAuraOwner:
		return true
	default:
		return false
	}
}

// spellEffectIsUnitOwnedAuraEffect mirrors
// SpellEffectInfo::IsUnitOwnedAuraEffect (SpellInfo.cpp:397-400): area-aura
// effects (SpellEffectInfo::IsAreaAuraEffect, SpellInfo.cpp:385-395) or
// SPELL_EFFECT_APPLY_AURA. It is the aura_effmask gate of the Spell::_cast
// diminishing-returns recheck (Spell.cpp:3378-3381).
func spellEffectIsUnitOwnedAuraEffect(eff wotlk.SpellEffect) bool {
	switch eff.Effect {
	case spellEffectApplyAreaAuraParty,
		spellEffectApplyAreaAuraRaid,
		spellEffectApplyAreaAuraFriend,
		spellEffectApplyAreaAuraEnemy,
		spellEffectApplyAreaAuraPet,
		spellEffectApplyAreaAuraOwner,
		spellEffectApplyAura:
		return true
	default:
		return false
	}
}

// spellHasEffect mirrors SpellInfo::HasEffect (SpellInfo.cpp:887-893):
// true when any spell effect carries the given effect id.
func spellHasEffect(spell wotlk.Spell, effectID uint32) bool {
	for _, eff := range spell.Effects {
		if eff.Effect == effectID {
			return true
		}
	}
	return false
}

// spellHasAura mirrors SpellInfo::HasAura (SpellInfo.cpp:890-896).
func spellHasAura(spell wotlk.Spell, aura uint32) bool {
	for _, eff := range spell.Effects {
		if eff.Aura == aura && spellEffectIsAuraEffect(eff) {
			return true
		}
	}
	return false
}

// spellSpecific mirrors SpellInfo::_LoadSpellSpecific (SpellInfo.cpp:2041-2235):
// the per-spell exclusivity classifier consumed by the spell-specific stack
// gates (SpellInfo.cpp:1398-1449) and by _LoadAuraState's seal term
// (SpellInfo.cpp:1971). firstRank resolves GetFirstRankSpell()->Id for the
// scroll branch (nil degrades to the spell's own ID, like a missing chain
// entry); the seal branch never touches it, so spellAuraState can pass nil.
func spellSpecific(spell wotlk.Spell, firstRank func(uint32) uint32) uint8 {
	switch spell.SpellFamilyName {
	case spellFamilyGeneric:
		// Food / Drinks (mostly)
		if spell.AuraInterruptFlags&auraInterruptFlagNotSeated != 0 {
			food, drink := false, false
			for _, eff := range spell.Effects {
				if !spellEffectIsAuraEffect(eff) {
					continue
				}
				switch eff.Aura {
				// Food
				case spellAuraModRegen, spellAuraObsModHealth:
					food = true
				// Drink
				case spellAuraModPowerRegen, spellAuraObsModPower:
					drink = true
				}
			}
			switch {
			case food && drink:
				return spellSpecificFoodAndDrink
			case food:
				return spellSpecificFood
			case drink:
				return spellSpecificDrink
			}
		} else {
			if firstRank != nil {
				switch firstRank(spell.ID) {
				case 8118, // Strength
					8099, // Stamina
					8112, // Spirit
					8096, // Intellect
					8115, // Agility
					8091: // Armor
					return spellSpecificScroll
				}
			}
			switch spell.ID {
			case 12880, // Enrage (Enrage)
				14201,
				14202,
				14203,
				14204,
				57518, // Enrage (Wrecking Crew)
				57519,
				57520,
				57521,
				57522,
				57514, // Enrage (Imp. Defensive Stance)
				57516:
				return spellSpecificWarriorEnrage
			}
		}
	case spellFamilyMage:
		// family flags 18(Molten), 25(Frost/Ice), 28(Mage)
		if spell.SpellFamilyFlags[0]&0x12040000 != 0 {
			return spellSpecificMageArmor
		}
		// Arcane brillance and Arcane intelect (normal check fails because of flags difference)
		if spell.SpellFamilyFlags[0]&0x400 != 0 {
			return spellSpecificMageArcaneBrillance
		}
		if spell.SpellFamilyFlags[0]&0x1000000 != 0 && spell.Effects[0].Aura == spellAuraConfuse {
			return spellSpecificMagePolymorph
		}
	case spellFamilyWarrior:
		if spell.ID == 12292 { // Death Wish
			return spellSpecificWarriorEnrage
		}
	case spellFamilyWarlock:
		// only warlock curses have this
		if spell.DispelType == DispelCurse {
			return spellSpecificCurse
		}
		// Warlock (Demon Armor | Demon Skin | Fel Armor)
		if spell.SpellFamilyFlags[1]&0x20000020 != 0 || spell.SpellFamilyFlags[2]&0x00000010 != 0 {
			return spellSpecificWarlockArmor
		}
		// seed of corruption and corruption
		if spell.SpellFamilyFlags[1]&0x10 != 0 || spell.SpellFamilyFlags[0]&0x2 != 0 {
			return spellSpecificWarlockCorruption
		}
	case spellFamilyPriest:
		// Divine Spirit and Prayer of Spirit
		if spell.SpellFamilyFlags[0]&0x20 != 0 {
			return spellSpecificPriestDivineSpirit
		}
	case spellFamilyHunter:
		// only hunter stings have this
		if spell.DispelType == DispelPoison {
			return spellSpecificSting
		}
		// only hunter aspects have this (but not all aspects in hunter family)
		if spell.SpellFamilyFlags[0]&0x00380000 != 0 || spell.SpellFamilyFlags[1]&0x00440000 != 0 || spell.SpellFamilyFlags[2]&0x00001010 != 0 {
			return spellSpecificAspect
		}
	case spellFamilyPaladin:
		// Collection of all the seal family flags. No other paladin spell has any of those.
		if spell.SpellFamilyFlags[1]&0x26000C00 != 0 || spell.SpellFamilyFlags[0]&0x0A000000 != 0 {
			return spellSpecificSeal
		}
		if spell.SpellFamilyFlags[0]&0x00002190 != 0 {
			return spellSpecificHand
		}
		// Judgement of Wisdom, Judgement of Light, Judgement of Justice
		switch spell.ID {
		case 20184, 20185, 20186:
			return spellSpecificJudgement
		}
		// only paladin auras have this (for palaldin class family)
		if spell.SpellFamilyFlags[2]&0x00000020 != 0 {
			return spellSpecificAura
		}
	case spellFamilyShaman:
		// family flags 10 (Lightning), 42 (Earth), 37 (Water), proc shield from T2 8 pieces bonus
		if spell.SpellFamilyFlags[1]&0x420 != 0 || spell.SpellFamilyFlags[0]&0x00000400 != 0 || spell.ID == 23552 {
			return spellSpecificElementalShield
		}
	case spellFamilyDeathKnight:
		if spell.ID == 48266 || spell.ID == 48263 || spell.ID == 48265 {
			return spellSpecificPresence
		}
	}
	for _, eff := range spell.Effects {
		if eff.Effect == spellEffectApplyAura {
			switch eff.Aura {
			case spellAuraCharm, spellAuraModPossessPet, spellAuraModPossess, spellAuraAoeCharm:
				return spellSpecificCharm
			case spellAuraTrackCreatures:
				/// @workaround For non-stacking tracking spells (We need generic solution)
				if spell.ID == 30645 { // Gas Cloud Tracking
					return spellSpecificNormal
				}
				return spellSpecificTracker
			case spellAuraTrackResources, spellAuraTrackStealthed:
				return spellSpecificTracker
			}
		}
	}
	return spellSpecificNormal
}

// isAuraExclusiveBySpecificWith mirrors SpellInfo::IsAuraExclusiveBySpecificWith
// (SpellInfo.cpp:1398-1429).
func isAuraExclusiveBySpecificWith(spec1, spec2 uint8) bool {
	switch spec1 {
	case spellSpecificTracker,
		spellSpecificWarlockArmor,
		spellSpecificMageArmor,
		spellSpecificElementalShield,
		spellSpecificMagePolymorph,
		spellSpecificPresence,
		spellSpecificCharm,
		spellSpecificScroll,
		spellSpecificWarriorEnrage,
		spellSpecificMageArcaneBrillance,
		spellSpecificPriestDivineSpirit:
		return spec1 == spec2
	case spellSpecificFood:
		return spec2 == spellSpecificFood || spec2 == spellSpecificFoodAndDrink
	case spellSpecificDrink:
		return spec2 == spellSpecificDrink || spec2 == spellSpecificFoodAndDrink
	case spellSpecificFoodAndDrink:
		return spec2 == spellSpecificFood || spec2 == spellSpecificDrink || spec2 == spellSpecificFoodAndDrink
	default:
		return false
	}
}

// isAuraExclusiveBySpecificPerCasterWith mirrors
// SpellInfo::IsAuraExclusiveBySpecificPerCasterWith (SpellInfo.cpp:1431-1449).
func isAuraExclusiveBySpecificPerCasterWith(spec1, spec2 uint8) bool {
	switch spec1 {
	case spellSpecificSeal,
		spellSpecificHand,
		spellSpecificAura,
		spellSpecificSting,
		spellSpecificCurse,
		spellSpecificAspect,
		spellSpecificJudgement,
		spellSpecificWarlockCorruption:
		return spec1 == spec2
	default:
		return false
	}
}

// spellSpecificNoStackPurge mirrors the "check spell specific stack rules" term
// of Aura::CanStackWith (SpellAuras.cpp:1912-1921) inside
// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640-3671): a fresh aura purges
// existing auras whose SpellSpecific classification is exclusive with the new
// spell's, or — with the same caster — whose per-caster specific matches. The
// TRACK_RESOURCES config term (SpellAuras.cpp:1912-1916) precedes the specific
// gates: when both auras carry SPELL_AURA_TRACK_RESOURCES they stack only if
// AllowTrackBothResources is set (C++ CONFIG_ALLOW_TRACK_BOTH_RESOURCES,
// worldserver.conf default false); otherwise the existing tracking aura is
// purged. The trigger-spell mutual exclusion (SpellAuras.cpp:1901-1906) is
// honored like the companion purge functions. Passive new spells skip the
// purge (the IsPassiveStackableWithRanks early-out shape, Unit.cpp:3643; Go
// holds no passive aura instances). Returns the auras to remove; the caller
// removes them after unlocking.
func (s *Server) spellSpecificNoStackPurge(newSpell wotlk.Spell, newCasterGUID uint64, existing map[uint32]*activeAura) []rankPurgeTarget {
	if s == nil || s.Data == nil {
		return nil
	}
	if newSpell.Attributes&spellAttributePassive != 0 {
		return nil
	}
	var purge []rankPurgeTarget
	// The config term applies regardless of classification, so it runs before
	// the spellSpecificNormal early return below.
	if !s.Config.AllowTrackBothResources && spellHasAura(newSpell, spellAuraTrackResources) {
		for id, aura := range existing {
			if id == newSpell.ID || aura == nil || aura.Stopped {
				continue
			}
			exSpell, found, err := s.Data.Spell(id)
			if err != nil || !found {
				continue
			}
			if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
				continue
			}
			if spellHasAura(exSpell, spellAuraTrackResources) {
				purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
			}
		}
	}
	newSpec := spellSpecific(newSpell, s.spellFirstRank)
	if newSpec == spellSpecificNormal {
		return purge
	}
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
			continue
		}
		// Tracking pairs were already decided by the config term above; C++
		// never evaluates the specific gates for them (SpellAuras.cpp:1916).
		if spellHasAura(newSpell, spellAuraTrackResources) && spellHasAura(exSpell, spellAuraTrackResources) {
			continue
		}
		exSpec := spellSpecific(exSpell, s.spellFirstRank)
		sameCaster := aura.CasterGUID == newCasterGUID
		if !isAuraExclusiveBySpecificWith(newSpec, exSpec) &&
			!(sameCaster && isAuraExclusiveBySpecificPerCasterWith(newSpec, exSpec)) {
			continue
		}
		purge = append(purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
	}
	return purge
}

// spellEffectIsAreaAura mirrors SpellEffectInfo::IsAreaAuraEffect
// (SpellInfo.cpp:385-395): the APPLY_AREA_AURA_* effect family
// (SharedDefines.h:846/876/930/939/940/954).
func spellEffectIsAreaAura(effect uint32) bool {
	switch effect {
	case 35, 65, 119, 128, 129, 143:
		return true
	default:
		return false
	}
}

// newSpellAuraEffectMask is the bit mask of the spell effects the aura
// sweep routes to applyAuraToTarget (eff.Effect == 6 || eff.Aura != 0) —
// the Go counterpart of the new aura's create-time effect mask used by the
// IsHighestExclusiveAuraEffect tie-break (Unit.cpp:14001).
func newSpellAuraEffectMask(spell wotlk.Spell) uint8 {
	var mask uint8
	for index, eff := range spell.Effects {
		if index >= 8 {
			break
		}
		if eff.Effect == 6 || eff.Aura != 0 {
			mask |= 1 << uint(index)
		}
	}
	return mask
}

func popcount8(v uint8) int {
	n := 0
	for v != 0 {
		n += int(v & 1)
		v >>= 1
	}
	return n
}

// exclusiveHighestVerdict mirrors Unit::IsHighestExclusiveAura and
// IsHighestExclusiveAuraEffect (Unit.cpp:13991-14036) for pairs whose
// CheckSpellGroupStackRules verdict is SPELL_GROUP_STACK_RULE_EXCLUSIVE_HIGHEST
// (SpellMgr.cpp:438-488, rule 4): for the new effect, every existing aura
// effect of the same aura type on the target is compared by absolute amount,
// with the effect-mask bit-count difference as the tie-break. A strictly
// higher new effect purges the existing aura — except an area aura owned by
// the target itself, which C++ never removes (SpellAuras.cpp:696,
// "no removing of area auras from the original owner, as that completely
// cancels them"); a strictly lower new effect suppresses the whole new aura
// (Unit.cpp:3648 / SpellAuras.cpp:696, addUnit=false); an exact tie purges
// the existing aura via the CanStackWith rule-4 term (SpellAuras.cpp:1928,
// "existing aura is lower/equal"), honoring the trigger-spell mutual
// exclusion (SpellAuras.cpp:1901-1906) like the companion purge functions.
// Purges already collected stay applied when a later comparison suppresses
// the new aura, matching C++'s immediate removals before its early return.
type exclusiveHighestVerdict struct {
	purge      []rankPurgeTarget
	suppressed bool
}

func (s *Server) exclusiveHighestVerdict(newSpell wotlk.Spell, eff wotlk.SpellEffect, newAmount int32, targetGUID uint64, existing map[uint32]*activeAura) exclusiveHighestVerdict {
	var out exclusiveHighestVerdict
	if s == nil || s.Data == nil {
		return out
	}
	if newSpell.Attributes&spellAttributePassive != 0 {
		return out
	}
	newFirst := s.spellFirstRank(newSpell.ID)
	newMask := newSpellAuraEffectMask(newSpell)
	newBits := popcount8(newMask)
	newAbs := absAuraAmount(newAmount)
	for id, aura := range existing {
		if id == newSpell.ID || aura == nil || aura.Stopped {
			continue
		}
		if s.spellGroupStackRule(newFirst, s.spellFirstRank(id)) != spellGroupStackRuleExclusiveHighest {
			continue
		}
		exSpell, found, err := s.Data.Spell(id)
		if err != nil || !found {
			continue
		}
		for index, exEff := range exSpell.Effects {
			if index >= 8 || exEff.Aura != eff.Aura {
				continue
			}
			if aura.EffectMask&(1<<uint(index)) == 0 {
				continue
			}
			diff := newAbs - absAuraAmount(aura.Amounts[index])
			if diff == 0 {
				diff = int64(newBits) - int64(popcount8(aura.EffectMask))
			}
			switch {
			case diff < 0:
				out.suppressed = true
				return out
			case diff > 0:
				if isExistingAreaAuraOfTarget(aura, exSpell, targetGUID) {
					continue
				}
				out.purge = append(out.purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
			default:
				if auraTriggersSpell(newSpell, id) || auraTriggersSpell(exSpell, newSpell.ID) {
					continue
				}
				out.purge = append(out.purge, rankPurgeTarget{spellID: id, slot: aura.Slot})
			}
		}
	}
	return out
}

// isExistingAreaAuraOfTarget reports the SpellAuras.cpp:696 area-aura guard:
// the existing aura carries an applied area-aura effect and its owner
// (the caster) is the target itself.
func isExistingAreaAuraOfTarget(aura *activeAura, exSpell wotlk.Spell, targetGUID uint64) bool {
	if aura == nil || aura.CasterGUID != targetGUID {
		return false
	}
	for index, exEff := range exSpell.Effects {
		if index >= 8 {
			break
		}
		if aura.EffectMask&(1<<uint(index)) != 0 && spellEffectIsAreaAura(exEff.Effect) {
			return true
		}
	}
	return false
}

// applyAuraToTarget applies one aura effect to a player target. casterGUID is
// the aura's caster: normally the casting session's player, but the
// spellsteal path passes the victim aura's original caster
// (Unit::RemoveAurasDueToSpellBySteal, Unit.cpp:4020:
// createInfo.SetCasterGUID(aura->GetCasterGUID())) — the no-stack purge's
// same-caster terms and the wire caster field key on it, not on the stealer.
func (s *session) applyAuraToTarget(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect, durationMs, periodMs, amount, schoolMask uint32, castMerged map[uint64]struct{}, skipSingleCastReg bool, casterGUID uint64) {
	if s.player == nil {
		return
	}
	channelTargetGUID := uint64(0)
	if eff.Aura == 23 && eff.TriggerSpell != 0 {
		channelTargetGUID = s.channelTargetForSpell(spell.ID)
	}
	if ctx == nil || ctx.Err() != nil {
		ctx = context.Background()
	}
	mountEffectMask, mountMiscValue, mountAmounts, mountBaseAmounts, mountedFlight := mountedFlightAuraEffects(spell)

	positive := !isHarmfulAura(eff.Aura) && eff.ImplicitTargetA != 6
	if isAreaEnemySpell(spell) {
		positive = false
	}

	// Target is a player (self or other online player)
	var targetSess *session
	if targetGUID == s.playerGUID || targetGUID == 0 {
		targetSess = s
		targetGUID = s.playerGUID
	} else if s.server != nil {
		targetSess = s.server.findSessionByGUID(targetGUID)
	}

	if targetSess != nil && targetSess.player != nil {
		if targetSess.player.Health == 0 {
			return
		}
		if targetSess.isImmuneToSpell(spell) {
			return
		}
		if eff.Aura == 36 {
			for _, existing := range targetSess.loadedAuras() {
				if existing != nil && existing.AuraType == 36 && existing.SpellID != spell.ID {
					targetSess.removeAura(existing.SpellID)
				}
			}
		}
		if eff.Aura == spellAuraMounted || mountedFlight {
			targetSess.clearOtherMountedAuras(spell.ID)
			durationMs = 0
			periodMs = 0
		}

		var drGroup DiminishingGroup
		if !positive && durationMs > 0 {
			var ok bool
			drGroup, durationMs, ok = targetSess.applyDiminishingToDuration(spell.ID, spell.Mechanic, durationMs, true)
			if !ok {
				// Target is immune to crowd control due to DR
				_ = s.write(uint16(protocol.OpcodeSMSG_CAST_FAILED), buildCastFailed(1, spell.ID, 38), true) // SPELL_FAILED_IMMUNE = 38
				return
			}
			targetSess.incrDiminishing(drGroup)
			targetSess.applyDiminishingAura(drGroup, true)
		}

		targetSess.castMu.Lock()
		if targetSess.auras == nil {
			targetSess.auras = make(map[uint32]struct{})
		}
		if targetSess.auraSlots == nil {
			targetSess.auraSlots = make(map[uint32]uint8)
		}
		if targetSess.activeAuras == nil {
			targetSess.activeAuras = make(map[uint32]*activeAura)
		}

		previous := targetSess.activeAuras[spell.ID]
		if existing := previous; existing != nil && !existing.Stopped {
			// Spell::DoSpellEffectHit (Spell.cpp:2842): only the first aura
			// effect per (cast, target) runs the merge — the first hit sets
			// hitInfo.HitAura and later effects only AddStaticApplication.
			// Go's per-effect sweep would otherwise run the ModStackAmount(+1)
			// merge once per aura effect, so a re-cast of a stackable
			// multi-effect aura would reach max stacks N× too fast. Later
			// effects only refresh this effect's basepoints/amounts.
			firstMerge := true
			if castMerged != nil {
				if _, ok := castMerged[targetGUID]; ok {
					firstMerge = false
				} else {
					castMerged[targetGUID] = struct{}{}
				}
			}
			if !firstMerge {
				refreshAuraEffectBasepoints(existing, spell, eff, amount)
				targetSess.castMu.Unlock()
				return
			}
			// Unit::_TryStackingOrRefreshingExistingAura (Unit.cpp:3326) ->
			// Aura::ModStackAmount(+1) (SpellAuras.cpp:1030): a re-cast merges
			// into the existing aura instead of replacing it — one more stack,
			// clamped to the spell's stack amount (1 when the spell is not
			// stackable). The +1 refresh always resets the charge count to the
			// spell's max charges and the duration to the new cast's duration
			// (DoSpellEffectHit, Spell.cpp:2899-2910). Go holds one aura per
			// spell ID, so the C++ caster-GUID match is vacuous.
			maxStack := int32(spell.StackAmount)
			if maxStack == 0 {
				maxStack = 1
			}
			cur := int32(existing.StackCount)
			if cur == 0 {
				cur = 1
			}
			if cur++; cur > maxStack {
				cur = maxStack
			}
			stackCount := uint8(cur)
			existing.StackCount = stackCount
			existing.RemainingCharges = uint8(spell.ProcCharges)
			// Unit::_TryStackingOrRefreshingExistingAura basepoint update
			// (Unit.cpp:3360-3372): a re-cast overwrites the aura's per-effect
			// basepoints with the new cast's values, and the live amounts are
			// recalculated from them (Aura::SetStackAmount, SpellAuras.cpp:1008).
			// Go has no stack-scaled amount recalc, so the amounts follow the
			// new cast directly; the stack-scaled multiplier half stays unmodeled.
			refreshAuraEffectBasepoints(existing, spell, eff, amount)
			existing.DurationMs = durationMs
			existing.RemainingMs = durationMs
			existing.DurationUpdatedAt = time.Now()
			// Aura::RefreshTimers(resetPeriodicTimer) (Spell.cpp:2854):
			// resetPeriodicTimer = StackAmount < 2 &&
			// !(triggeredCastFlags & TRIGGERED_DONT_RESET_PERIODIC_TIMER).
			// Every C++ triggered cast carries the bit — TRIGGERED_FULL_MASK
			// (0x0007FFFF) includes TRIGGERED_DONT_RESET_PERIODIC_TIMER
			// (0x00020000, SpellDefines.h:151) — so a triggered re-cast of a
			// non-stackable aura keeps the periodic tick countdown alive
			// (AuraEffect::CalculatePeriodic, SpellAuraEffects.cpp:631). The
			// triggered-cast funnel bumps triggeredNoProcEvents, so the
			// counter doubles as the FULL_MASK marker here; the
			// client-initiated path never sets it.
			resetPeriodic := spell.StackAmount < 2 && s.triggeredNoProcEvents == 0
			if existing.Timer != nil {
				existing.Timer.Stop()
				existing.Timer = nil
			}
			if resetPeriodic {
				if existing.TickTimer != nil {
					existing.TickTimer.Stop()
					existing.TickTimer = nil
				}
				existing.PeriodMs = periodMs
			}
			slot, effectMask, charges := existing.Slot, existing.EffectMask, existing.RemainingCharges
			targetSess.castMu.Unlock()
			// AuraEffect::ChangeAmount -> CalculateSpellMod
			// (SpellAuraEffects.cpp:657-683): the merged aura's refreshed
			// amounts update the registered modifier values, no re-register.
			targetSess.refreshSpellModValues(existing)
			if resetPeriodic && periodMs > 0 {
				targetSess.schedulePlayerPeriodicTick(existing, periodMs)
			}
			if durationMs > 0 && durationMs < 18000000 {
				targetSess.castMu.Lock()
				existing.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
					targetSess.expirePlayerAura(spell.ID)
				})
				targetSess.castMu.Unlock()
			}
			// Charges ride the stack-count field of the aura update
			// (player_auras.go), same convention as the fresh-apply path.
			wireStack := stackCount
			if !mountedFlight && eff.Aura != spellAuraMounted && spell.StackAmount == 0 && spell.ProcCharges > 0 {
				wireStack = charges
			}
			wireMaxDuration, wireDuration := auraWireDurations(spell, durationMs, durationMs)
			updatePkt := protocol.BuildAuraUpdateWithStackEffect(targetGUID, casterGUID, slot, spell.ID, false, positive, wireMaxDuration, wireDuration, s.player.Level, wireStack, effectMask)
			_ = targetSess.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
			if s.server != nil {
				s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, targetSess)
			}
			targetSess.sendPlayerUpdate()
			return
		}
		if existing := previous; existing != nil {
			existing.Stopped = true
			if existing.Timer != nil {
				existing.Timer.Stop()
			}
			if existing.TickTimer != nil {
				existing.TickTimer.Stop()
			}
			if existing.DRGroup != DiminishingNone {
				targetSess.applyDiminishingAura(existing.DRGroup, false)
				existing.DRGroup = DiminishingNone
			}
		}

		// Unit::IsHighestExclusiveAura (Unit.cpp:13991): a fresh aura whose
		// effect is strictly lower than an existing EXCLUSIVE_HIGHEST peer
		// is never applied (SpellAuras.cpp:696, addUnit=false; Unit.cpp:3648
		// removes it before the no-stack purge). The strictly-higher purges
		// are applied below with the other no-stack purges.
		highest := s.server.exclusiveHighestVerdict(spell, eff, int32(amount), targetGUID, targetSess.activeAuras)
		if highest.suppressed {
			targetSess.castMu.Unlock()
			return
		}

		targetSess.auras[spell.ID] = struct{}{}
		slot, ok := targetSess.auraSlots[spell.ID]
		if !ok {
			used := make(map[uint8]bool)
			for _, sl := range targetSess.auraSlots {
				used[sl] = true
			}
			var freeSlot uint8
			for sl := uint8(0); sl < 64; sl++ {
				if !used[sl] {
					freeSlot = sl
					break
				}
			}
			slot = freeSlot
			targetSess.auraSlots[spell.ID] = slot
		}

		aura := &activeAura{
			SpellID:            spell.ID,
			DispelType:         spell.DispelType,
			Mechanic:           spell.Mechanic,
			AuraType:           eff.Aura,
			EffectMask:         spellEffectMask(spell, eff),
			CasterGUID:         casterGUID,
			TargetGUID:         targetGUID,
			ChannelTargetGUID:  channelTargetGUID,
			SchoolMask:         schoolMask,
			MiscValue:          eff.MiscValue,
			Amount:             amount,
			DurationMs:         durationMs,
			PeriodMs:           periodMs,
			RemainingMs:        durationMs,
			DurationUpdatedAt:  time.Now(),
			Slot:               slot,
			Positive:           positive,
			CasterLevel:        s.player.Level,
			SingleTarget:       isSingleTargetAuraSpell(spell),
			AuraInterruptFlags: spell.AuraInterruptFlags,
			TriggerSpell:       eff.TriggerSpell,
			DRGroup:            drGroup,
			StackAmount:        spell.StackAmount,
			HideDuration:       spell.AttributesEx5&spellAttr5HideDuration != 0,
			RemainingCharges:   uint8(spell.ProcCharges),
		}
		setAuraEffectPersistence(aura, spell, eff, amount)
		if eff.Aura == 4 && previous != nil && previous.AuraType == 4 {
			currentMask := spellEffectMask(spell, eff)
			aura.EffectMask |= previous.EffectMask
			for index := range spell.Effects {
				bit := uint8(1 << uint(index))
				if previous.EffectMask&bit == 0 || currentMask&bit != 0 {
					continue
				}
				aura.Amounts[index], aura.BaseAmounts[index] = previous.Amounts[index], previous.BaseAmounts[index]
				aura.RecalculateMask |= previous.RecalculateMask & bit
			}
		}
		if previous != nil && (previous.AuraType == 36 || eff.Aura == 36) {
			aura.EffectMask |= previous.EffectMask
			aura.Amounts, aura.BaseAmounts = previous.Amounts, previous.BaseAmounts
			aura.RecalculateMask |= previous.RecalculateMask
			if previous.AuraType == 36 && eff.Aura != 36 {
				aura.AuraType, aura.MiscValue = previous.AuraType, previous.MiscValue
			}
		}
		if mountedFlight {
			aura.EffectMask |= mountEffectMask
			aura.AuraType = spellAuraMounted
			aura.MiscValue = mountMiscValue
			aura.Positive = true
			aura.DurationMs, aura.RemainingMs, aura.PeriodMs = 0, 0, 0
			aura.DRGroup, aura.StackCount, aura.RemainingCharges = DiminishingNone, 1, 0
			aura.RecalculateMask &^= mountEffectMask
			for index, effect := range spell.Effects {
				bit := uint8(1 << uint(index))
				if mountEffectMask&bit == 0 {
					continue
				}
				aura.Amounts[index], aura.BaseAmounts[index] = mountAmounts[index], mountBaseAmounts[index]
				if effect == eff {
					aura.Amounts[index] = int32(amount)
				}
				if auraEffectCanBeRecalculated(effect.Aura) {
					aura.RecalculateMask |= bit
				}
				if effect.Aura == spellAuraMounted {
					aura.Amount = uint32(aura.Amounts[index])
				}
			}
		}
		targetSess.activeAuras[spell.ID] = aura
		// Unit::_AddAura single-target dance (Unit.cpp:3397-3420): a fresh
		// single-target aura registers on its caster's single-cast list and
		// purges the caster's other single-target-with auras on other
		// targets. Steal-created auras skip registration (Unit.cpp:4028).
		var scPurge []singleCastEntry
		if !skipSingleCastReg && spellIsSingleTarget(spell) {
			scPurge = s.server.registerSingleCastAura(spell, aura)
		}
		// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640): the fresh
		// aura purges auras of other spells it can't stack with — the
		// rank-chain term (Aura::CanStackWith, SpellAuras.cpp:1994-2004),
		// the spell-group exclusive terms (SpellAuras.cpp:1924-1932),
		// the spell-specific exclusivity gates (SpellAuras.cpp:1914-1921),
		// and the EXCLUSIVE_HIGHEST comparisons (Unit.cpp:13991) — plus
		// the _AddAura single-target dance (Unit.cpp:3397-3420) for
		// single-target auras. The purge keys on the new aura's caster
		// (the same-caster terms compare against it), which is the
		// casterGUID param — not the casting session — because the steal
		// and dynamic-object paths create the aura with a foreign caster
		// (Unit::RemoveAurasDueToSpellBySteal, Unit.cpp:4020:
		// createInfo.SetCasterGUID(aura->GetCasterGUID())).
		purgeIDs := s.server.rankChainNoStackPurge(spell, casterGUID, aura.ItemGUID, targetSess.activeAuras)
		purgeIDs = append(purgeIDs, s.server.spellGroupNoStackPurge(spell, casterGUID, targetSess.activeAuras)...)
		purgeIDs = append(purgeIDs, s.server.spellSpecificNoStackPurge(spell, casterGUID, targetSess.activeAuras)...)
		purgeIDs = append(purgeIDs, s.server.singleTargetNoStackPurge(spell, casterGUID, targetSess.activeAuras)...)
		purgeIDs = append(purgeIDs, highest.purge...)
		targetSess.castMu.Unlock()
		// AuraEffect::ApplySpellMod on fresh apply / replace
		// (SpellAuraEffects.cpp:755): the replaced aura's modifiers leave
		// before the new aura's register.
		targetSess.dropSpellMods(spell.ID)
		targetSess.addSpellMods(aura)
		for _, purgeID := range purgeIDs {
			targetSess.expirePlayerAura(purgeID.spellID)
		}
		for _, e := range scPurge {
			s.expireSingleCastEntry(e)
		}
		if eff.Aura == spellAuraModParryPercent {
			targetSess.updatePlayerParryPercentage(targetSess.player, targetSess.player.Level)
		}
		if eff.Aura == spellAuraFakeInebriation {
			targetSess.player.FakeInebriation += amount
		}
		if eff.Aura == spellAuraStun || eff.Aura == spellAuraRoot {
			targetSess.rooted = true
			if eff.Aura == spellAuraStun {
				targetSess.player.UnitFlags |= unitFlagStunned
			}
			targetSess.sendForcedMovement(uint16(protocol.OpcodeSMSG_FORCE_MOVE_ROOT))
		}
		if eff.Aura == spellAuraConfuse || eff.Aura == spellAuraFear {
			targetSess.interruptCurrentCast()
			targetSess.interruptCurrentChannel()
			if targetSess.attackTarget != 0 {
				_ = targetSess.handleAttackStop()
			}
			if eff.Aura == spellAuraConfuse {
				targetSess.player.UnitFlags |= unitFlagConfused
			} else {
				targetSess.player.UnitFlags |= unitFlagFleeing
			}
		}
		if eff.Aura == spellAuraCharm {
			targetSess.clearOtherMountedAuras(0)
			targetSess.interruptCurrentCast()
			targetSess.interruptCurrentChannel()
			if targetSess.attackTarget != 0 {
				_ = targetSess.handleAttackStop()
			}
			targetSess.sendClientControl(targetSess.playerGUID, false)
		}
		if eff.Aura == 139 {
			_ = targetSess.sendForcedReactions()
			if eff.MiscValue >= 0 && amount >= 4 {
				targetSess.stopAttacksForFaction(ctx, uint32(eff.MiscValue))
			}
		}
		if eff.Aura == spellAuraMounted {
			targetSess.applyMountedDisplay(ctx, aura)
			aura.StackCount = 1
			aura.RemainingCharges = 0
		}
		if eff.Aura == spellAuraStealth {
			targetSess.player.StandFlags |= unitStandFlagCreep
			targetSess.player.AuraVision |= playerAuraVisionStealth
		}
		if eff.Aura == spellAuraInvisibility {
			targetSess.player.AuraVision |= playerAuraVisionInvis
		}
		if eff.Aura == spellAuraTrackStealthed {
			targetSess.player.PlayerFieldBytes |= playerFieldByteTrackStealthed
		}
		if eff.Aura == 36 || eff.Aura == 56 {
			targetSess.refreshTransformDisplay(ctx)
		}

		stackCount := uint8(1)
		if !mountedFlight && eff.Aura != spellAuraMounted && spell.StackAmount == 0 && spell.ProcCharges > 0 {
			stackCount = uint8(spell.ProcCharges)
		}
		wireMaxDuration, wireDuration := auraWireDurations(spell, durationMs, durationMs)
		updatePkt := protocol.BuildAuraUpdateWithStackEffect(targetGUID, s.playerGUID, slot, spell.ID, false, positive, wireMaxDuration, wireDuration, s.player.Level, stackCount, spellEffectMask(spell, eff))
		_ = targetSess.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
		if s.server != nil {
			s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, targetSess)
		}
		targetSess.sendPlayerUpdate()
		if eff.Aura == 4 {
			targetSess.addOwnerPetAuraEffects(ctx, spell.ID, spellEffectMask(spell, eff))
		}
		if movementSpeedAura(eff.Aura) {
			targetSess.sendRuntimeMovementUpdates(eff.Aura)
		}
		if affectsPlayerVisibility(eff.Aura) && targetSess.server != nil {
			targetSess.server.refreshPlayerVisibility()
		}

		if periodMs > 0 {
			targetSess.schedulePlayerPeriodicTick(aura, periodMs)
		}
		if durationMs > 0 && durationMs < 18000000 {
			targetSess.castMu.Lock()
			aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
				targetSess.expirePlayerAura(spell.ID)
			})
			targetSess.castMu.Unlock()
		}
		return
	}

	// Target is a creature in the world
	target, ok := s.getCombatTarget(ctx, targetGUID)
	if !ok || target.Health == 0 {
		return
	}
	targetKey := creatureAuraKeyForTarget(target)

	if s.server == nil || s.server.isCreatureEvadingInInstance(s.player.Map, s.player.InstanceID, targetGUID) {
		return
	}
	s.server.auraMu.Lock()
	if s.server.creatureAuras == nil {
		s.server.creatureAuras = make(map[creatureAuraKey]map[uint32]struct{})
	}
	if s.server.creatureAuras[targetKey] == nil {
		s.server.creatureAuras[targetKey] = make(map[uint32]struct{})
	}
	s.server.creatureAuras[targetKey][spell.ID] = struct{}{}

	if s.server.activeCreatureAuras == nil {
		s.server.activeCreatureAuras = make(map[creatureAuraKey]map[uint32]*activeAura)
	}
	if s.server.activeCreatureAuras[targetKey] == nil {
		s.server.activeCreatureAuras[targetKey] = make(map[uint32]*activeAura)
	}
	if existing, exists := s.server.activeCreatureAuras[targetKey][spell.ID]; exists && existing != nil && !existing.Stopped {
		// Same ModStackAmount(+1) merge as the player path above
		// (Unit.cpp:3326, SpellAuras.cpp:1030). Per Spell::DoSpellEffectHit
		// (Spell.cpp:2842) only the first aura effect per (cast, target)
		// merges — castMerged carries the first-merge marker, later effects
		// only refresh this effect's basepoints/amounts.
		firstMerge := true
		if castMerged != nil {
			if _, ok := castMerged[targetGUID]; ok {
				firstMerge = false
			} else {
				castMerged[targetGUID] = struct{}{}
			}
		}
		if !firstMerge {
			refreshAuraEffectBasepoints(existing, spell, eff, amount)
			s.server.auraMu.Unlock()
			return
		}
		maxStack := int32(spell.StackAmount)
		if maxStack == 0 {
			maxStack = 1
		}
		cur := int32(existing.StackCount)
		if cur == 0 {
			cur = 1
		}
		if cur++; cur > maxStack {
			cur = maxStack
		}
		stackCount := uint8(cur)
		existing.StackCount = stackCount
		existing.RemainingCharges = uint8(spell.ProcCharges)
		// Creature-side mirror of the player merge's basepoint update
		// (Unit.cpp:3360-3372, Aura::SetStackAmount, SpellAuras.cpp:1008).
		refreshAuraEffectBasepoints(existing, spell, eff, amount)
		existing.DurationMs = durationMs
		existing.RemainingMs = durationMs
		existing.DurationUpdatedAt = time.Now()
		// Creature-side mirror of the player merge above: a triggered re-cast
		// keeps the periodic tick countdown (Spell.cpp:2854 —
		// TRIGGERED_DONT_RESET_PERIODIC_TIMER rides TRIGGERED_FULL_MASK on
		// the Go triggered-cast funnel too).
		resetPeriodic := spell.StackAmount < 2 && s.triggeredNoProcEvents == 0
		if existing.Timer != nil {
			existing.Timer.Stop()
			existing.Timer = nil
		}
		if resetPeriodic {
			if existing.TickTimer != nil {
				existing.TickTimer.Stop()
				existing.TickTimer = nil
			}
			existing.PeriodMs = periodMs
		}
		slot, effectMask := existing.Slot, existing.EffectMask
		s.server.auraMu.Unlock()
		if resetPeriodic && periodMs > 0 {
			s.scheduleCreaturePeriodicTick(existing, periodMs)
		}
		if durationMs > 0 && durationMs < 18000000 {
			s.server.auraMu.Lock()
			existing.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
				s.expireCreatureAura(targetKey, spell.ID, slot)
			})
			s.server.auraMu.Unlock()
		}
		wireStack := stackCount
		if spell.StackAmount == 0 && spell.ProcCharges > 0 {
			wireStack = uint8(spell.ProcCharges)
		}
		wireMaxDuration, wireDuration := auraWireDurations(spell, durationMs, durationMs)
		updatePkt := protocol.BuildAuraUpdateWithStackEffect(targetGUID, s.playerGUID, slot, spell.ID, false, positive, wireMaxDuration, wireDuration, s.player.Level, wireStack, effectMask)
		_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
		s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, s)
		return
	}
	if existing, exists := s.server.activeCreatureAuras[targetKey][spell.ID]; exists && existing != nil {
		existing.Stopped = true
		if existing.Timer != nil {
			existing.Timer.Stop()
		}
		if existing.TickTimer != nil {
			existing.TickTimer.Stop()
		}
	}

	// Unit::IsHighestExclusiveAura (Unit.cpp:13991): a fresh aura whose
	// effect is strictly lower than an existing EXCLUSIVE_HIGHEST peer
	// is never applied (SpellAuras.cpp:696, addUnit=false; Unit.cpp:3648
	// removes it before the no-stack purge). The strictly-higher purges
	// are applied below with the other no-stack purges.
	highest := s.server.exclusiveHighestVerdict(spell, eff, int32(amount), targetGUID, s.server.activeCreatureAuras[targetKey])
	if highest.suppressed {
		s.server.auraMu.Unlock()
		return
	}

	slot := uint8(len(s.server.activeCreatureAuras[targetKey]) % 64)
	aura := &activeAura{
		SpellID:           spell.ID,
		DispelType:        spell.DispelType,
		Mechanic:          spell.Mechanic,
		AuraType:          eff.Aura,
		EffectMask:        spellEffectMask(spell, eff),
		CasterGUID:        s.playerGUID,
		TargetGUID:        targetGUID,
		TargetKey:         targetKey,
		SchoolMask:        schoolMask,
		MiscValue:         eff.MiscValue,
		Amount:            amount,
		DurationMs:        durationMs,
		PeriodMs:          periodMs,
		RemainingMs:       durationMs,
		DurationUpdatedAt: time.Now(),
		Slot:              slot,
		Positive:          positive,
		CasterLevel:       s.player.Level,
		SingleTarget:      isSingleTargetAuraSpell(spell),
		TriggerSpell:      eff.TriggerSpell,
		StackAmount:       spell.StackAmount,
		HideDuration:      spell.AttributesEx5&spellAttr5HideDuration != 0,
		RemainingCharges:  uint8(spell.ProcCharges),
	}
	s.server.activeCreatureAuras[targetKey][spell.ID] = aura
	// Unit::_AddAura single-target dance (Unit.cpp:3397-3420): a fresh
	// single-target aura registers on its caster's single-cast list and
	// purges the caster's other single-target-with auras on other targets.
	// Steal-created auras skip registration (Unit.cpp:4028).
	var scPurge []singleCastEntry
	if !skipSingleCastReg && spellIsSingleTarget(spell) {
		scPurge = s.server.registerSingleCastAura(spell, aura)
	}
	// Unit::_RemoveNoStackAurasDueToAura (Unit.cpp:3640): the fresh aura
	// purges auras of other spells it can't stack with — the rank-chain
	// term (Aura::CanStackWith, SpellAuras.cpp:1994-2004), the spell-group
	// exclusive terms (SpellAuras.cpp:1924-1932), the spell-specific
	// exclusivity gates (SpellAuras.cpp:1914-1921), and the EXCLUSIVE_HIGHEST
	// comparisons (Unit.cpp:13991) — plus the _AddAura single-target dance
	// (Unit.cpp:3397-3420) for single-target auras. The purge keys on the
	// new aura's caster — the casterGUID param — because the
	// dynamic-object path creates the aura with the object's caster
	// (Unit.cpp:4020 shape).
	purge := s.server.rankChainNoStackPurge(spell, casterGUID, aura.ItemGUID, s.server.activeCreatureAuras[targetKey])
	purge = append(purge, s.server.spellGroupNoStackPurge(spell, casterGUID, s.server.activeCreatureAuras[targetKey])...)
	purge = append(purge, s.server.spellSpecificNoStackPurge(spell, casterGUID, s.server.activeCreatureAuras[targetKey])...)
	purge = append(purge, s.server.singleTargetNoStackPurge(spell, casterGUID, s.server.activeCreatureAuras[targetKey])...)
	purge = append(purge, highest.purge...)
	s.server.auraMu.Unlock()
	for _, p := range purge {
		s.expireCreatureAura(targetKey, p.spellID, p.slot)
	}
	for _, e := range scPurge {
		s.expireSingleCastEntry(e)
	}
	if eff.Aura == spellAuraCharm {
		spells, reactState, commandState, controlled := s.server.charmCreature(ctx, targetKey, s.playerGUID, s.player.Race)
		if controlled {
			s.sendClientControl(targetGUID, true)
			s.sendCharmPetSpells(targetGUID, spells, reactState, commandState)
		}
	}

	stackCount := uint8(1)
	if spell.StackAmount == 0 && spell.ProcCharges > 0 {
		stackCount = uint8(spell.ProcCharges)
	}
	wireMaxDuration, wireDuration := auraWireDurations(spell, durationMs, durationMs)
	updatePkt := protocol.BuildAuraUpdateWithStackEffect(targetGUID, s.playerGUID, slot, spell.ID, false, positive, wireMaxDuration, wireDuration, s.player.Level, stackCount, spellEffectMask(spell, eff))
	_ = s.write(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, true)
	s.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_AURA_UPDATE), updatePkt, s)

	if periodMs > 0 {
		s.scheduleCreaturePeriodicTick(aura, periodMs)
	}
	if durationMs > 0 && durationMs < 18000000 {
		s.server.auraMu.Lock()
		aura.Timer = time.AfterFunc(time.Duration(durationMs)*time.Millisecond, func() {
			s.expireCreatureAura(targetKey, spell.ID, slot)
		})
		s.server.auraMu.Unlock()
	}
}

func affectsPlayerVisibility(auraType uint32) bool {
	switch auraType {
	case spellAuraStealth, spellAuraInvisibility, spellAuraStealthDetect, spellAuraInvisibilityDetect, spellAuraStealthLevel:
		return true
	default:
		return false
	}
}

func AffectsPlayerVisibility(auraType uint32) bool {
	return affectsPlayerVisibility(auraType)
}

func (ts *session) schedulePlayerPeriodicTick(aura *activeAura, periodMs uint32) {
	ts.castMu.Lock()
	ts.schedulePlayerPeriodicTickLocked(aura, periodMs)
	ts.castMu.Unlock()
}

func (ts *session) schedulePlayerPeriodicTickLocked(aura *activeAura, periodMs uint32) {
	aura.TickTimer = time.AfterFunc(time.Duration(periodMs)*time.Millisecond, func() {
		ts.castMu.Lock()
		if aura.Stopped || ts.player == nil || ts.player.Health == 0 {
			ts.castMu.Unlock()
			return
		}
		advanceAuraDuration(aura, time.Now())
		stillRunning := aura.RemainingMs > 0 || aura.DurationMs == 0
		ts.castMu.Unlock()

		ts.executePeriodicTickOnPlayer(aura)

		if stillRunning {
			ts.castMu.Lock()
			if !aura.Stopped {
				ts.schedulePlayerPeriodicTickLocked(aura, periodMs)
			}
			ts.castMu.Unlock()
		}
	})
}

func (ts *session) executePeriodicTickOnPlayer(aura *activeAura) {
	if aura.AuraType == 23 && aura.TriggerSpell != 0 {
		ts.castSpellDirect(context.Background(), aura.TriggerSpell, ts.periodicTriggerTarget(aura))
		return
	}
	ts.playerStateMu.Lock()
	defer ts.playerStateMu.Unlock()
	if ts.player == nil || ts.player.Health == 0 {
		return
	}

	switch aura.AuraType {
	case 3, 89: // SPELL_AURA_PERIODIC_DAMAGE, SPELL_AURA_PERIODIC_DAMAGE_PERCENT
		dmg := aura.Amount
		resisted := uint32(0)
		if aura.SchoolMask&1 != 0 && ts.player.Armor > 0 {
			dmg = calcArmorReducedDamage(float64(ts.player.Armor), aura.CasterLevel, dmg)
		} else if aura.SchoolMask > 1 && aura.CasterLevel > 0 {
			pen := uint32(0)
			if ts.server != nil {
				if cs := ts.server.findSessionByGUID(aura.CasterGUID); cs != nil && cs.player != nil {
					pen = cs.player.SpellPenetration
				}
			}
			resisted, dmg = calcMagicSpellResistance(dmg, uint8(aura.SchoolMask), ts.player.Resistances, aura.CasterLevel, ts.player.Level, false, false, pen)
		}
		if aura.CasterGUID != aura.TargetGUID {
			ts.applyResilienceToDamage(true, &dmg, false, CombatRatingCritTakenSpell)
		}
		if dmg < 1 && resisted == 0 {
			dmg = 1
		}
		absorbed := uint32(0)
		if dmg > 0 {
			absorbed, dmg = ts.applyAbsorptionShields(dmg, uint8(aura.SchoolMask))
		}
		targetHealth := ts.player.Health
		overkill := uint32(0)
		if dmg >= targetHealth && targetHealth > 0 {
			overkill = dmg - targetHealth
		}

		logPkt := protocol.BuildPeriodicAuraLogDamage(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, dmg, overkill, aura.SchoolMask, absorbed, resisted, false)
		_ = ts.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if ts.server != nil {
			if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil && casterSess != ts {
				_ = casterSess.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
			}
			ts.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, ts)
		}

		if dmg >= targetHealth {
			if ts.duelPartner != 0 && ts.player.DuelTeam != 0 {
				ts.player.Health = 1
				ts.sendPlayerUpdate()
				if ts.server != nil {
					if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil {
						casterSess.endDuel(true, casterSess.playerGUID, false)
					}
				}
			} else {
				ts.player.Health = 0
				ts.sendPlayerUpdate()
				ts.killPlayer(context.Background())
				// Eluna CREATURE_EVENT_ON_TARGET_DIED (3): C++ Unit::Kill pet
				// arm — the periodic tick's attacker is the aura caster (a
				// player in Go's model), so only the caster's live pet gets
				// KilledUnit(victim) (Unit.cpp:11324-11335).
				if ts.server != nil {
					if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil {
						if pet := casterSess.livePetMotion(); pet != nil {
							ts.server.fireCreatureTargetDied(context.Background(), pet, ts.luaPlayer())
						}
					}
				}
			}
			ts.clearActiveAuras()
		} else {
			ts.player.Health -= dmg
			ts.procDamageAuras(false, dmg)
			ts.sendPlayerUpdate()
		}

	case 8, 20: // SPELL_AURA_PERIODIC_HEAL, SPELL_AURA_OBS_MOD_HEALTH
		heal := aura.Amount
		curHP := ts.player.Health
		maxHP := ts.player.MaxHealth
		newHP := curHP + heal
		overheal := uint32(0)
		if newHP > maxHP {
			overheal = newHP - maxHP
			newHP = maxHP
		}

		logPkt := protocol.BuildPeriodicAuraLogHeal(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, heal, overheal, 0, false)
		_ = ts.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if ts.server != nil {
			if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil && casterSess != ts {
				_ = casterSess.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
			}
			ts.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, ts)
		}

		ts.player.Health = newHP
		ts.sendPlayerUpdate()

		if ts.server != nil && heal > overheal {
			ts.server.distributeHealingThreat(context.Background(), aura.CasterGUID, aura.TargetGUID, heal-overheal)
		}

	case 24: // SPELL_AURA_PERIODIC_ENERGIZE
		powerType := uint32(aura.MiscValue)
		logPkt := protocol.BuildPeriodicAuraLogEnergize(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, powerType, aura.Amount)
		_ = ts.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if ts.server != nil {
			if casterSess := ts.server.findSessionByGUID(aura.CasterGUID); casterSess != nil && casterSess != ts {
				_ = casterSess.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
			}
			ts.server.broadcastToNearby(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, ts)
		}
		ts.adjustSpellPower(context.Background(), aura.TargetGUID, aura.MiscValue, int64(aura.Amount))
	}
}

func (s *session) channelTargetForSpell(spellID uint32) uint64 {
	if s == nil {
		return 0
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	if channel := s.activeChannel; channel != nil && !channel.Stopped && channel.SpellID == spellID {
		return channel.TargetGUID
	}
	return 0
}

func (s *session) periodicTriggerTarget(aura *activeAura) uint64 {
	if aura == nil {
		return 0
	}
	if aura.ChannelTargetGUID != 0 {
		return aura.ChannelTargetGUID
	}
	if targetGUID := s.channelTargetForSpell(aura.SpellID); targetGUID != 0 {
		return targetGUID
	}
	return aura.TargetGUID
}

func (ts *session) expirePlayerAura(spellID uint32) {
	ts.castMu.Lock()
	if ts.activeAuras != nil {
		if aura, ok := ts.activeAuras[spellID]; ok && aura != nil {
			aura.Stopped = true
			if aura.TickTimer != nil {
				aura.TickTimer.Stop()
			}
			delete(ts.activeAuras, spellID)
			ts.server.unregisterSingleCastAura(aura)
		}
	}
	ts.castMu.Unlock()
	ts.dropSpellMods(spellID)
	ts.removeAura(spellID)
}

func (s *session) scheduleCreaturePeriodicTick(aura *activeAura, periodMs uint32) {
	if s.server == nil {
		return
	}
	s.server.auraMu.Lock()
	s.scheduleCreaturePeriodicTickLocked(aura, periodMs)
	s.server.auraMu.Unlock()
}

func (s *session) scheduleCreaturePeriodicTickLocked(aura *activeAura, periodMs uint32) {
	aura.TickTimer = time.AfterFunc(time.Duration(periodMs)*time.Millisecond, func() {
		if s.server == nil {
			return
		}
		s.server.auraMu.Lock()
		if aura.Stopped {
			s.server.auraMu.Unlock()
			return
		}
		advanceAuraDuration(aura, time.Now())
		stillRunning := aura.RemainingMs > 0 || aura.DurationMs == 0
		s.server.auraMu.Unlock()

		targetAlive := s.executePeriodicTickOnCreature(aura)
		if !targetAlive {
			return
		}

		if stillRunning {
			s.server.auraMu.Lock()
			if !aura.Stopped {
				s.scheduleCreaturePeriodicTickLocked(aura, periodMs)
			}
			s.server.auraMu.Unlock()
		}
	})
}

func (s *session) executePeriodicTickOnCreature(aura *activeAura) bool {
	ctx := context.Background()
	key := aura.TargetKey
	target, ok := s.getCombatTarget(ctx, aura.TargetGUID)
	if !ok || target.Map != key.Map || target.InstanceID != key.InstanceID || target.Health == 0 || (s.server != nil && s.server.isCreatureEvadingInInstance(key.Map, key.InstanceID, key.GUID)) {
		if s.server != nil {
			s.server.clearCreatureAuras(key)
		}
		return false
	}

	switch aura.AuraType {
	case 3, 89: // SPELL_AURA_PERIODIC_DAMAGE, SPELL_AURA_PERIODIC_DAMAGE_PERCENT
		dmg := aura.Amount
		resisted := uint32(0)
		if aura.SchoolMask&1 != 0 && target.Armor > 0 {
			dmg = calcArmorReducedDamage(float64(target.Armor), aura.CasterLevel, dmg)
		} else if aura.SchoolMask > 1 && aura.CasterLevel > 0 {
			pen := uint32(0)
			if s.server != nil {
				if cs := s.server.findSessionByGUID(aura.CasterGUID); cs != nil && cs.player != nil {
					pen = cs.player.SpellPenetration
				}
			}
			resisted, dmg = calcMagicSpellResistance(dmg, uint8(aura.SchoolMask), target.Resistances, aura.CasterLevel, target.Level, true, false, pen)
		}
		if dmg < 1 && resisted == 0 {
			dmg = 1
		}
		targetHealth := target.Health
		overkill := uint32(0)
		if dmg >= targetHealth && targetHealth > 0 {
			overkill = dmg - targetHealth
		}

		logPkt := protocol.BuildPeriodicAuraLogDamage(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, dmg, overkill, aura.SchoolMask, 0, resisted, false)
		_ = s.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if s.server != nil {
			s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, s)
		}

		// Eluna CREATURE_EVENT_ON_DAMAGE_TAKEN (9): fires before damage
		// apply; handlers may rewrite damage via the second return
		// (Unit::DealDamage, Unit.cpp:697-702). Fired after the periodic
		// aura log, matching C++ sending the log before DealDamage
		// (SpellAuraEffects.cpp:5222-5224). The tick floors damage at 1
		// above, so the C++ all-zero DealDamage skip never applies here.
		if s.server != nil {
			var attacker *scripting.Object
			if aura.CasterGUID == s.playerGUID {
				attacker = s.luaPlayer()
			} else if cs := s.server.findSessionByGUID(aura.CasterGUID); cs != nil {
				attacker = cs.luaPlayer()
			}
			if attacker != nil {
				if motion := s.server.findCreatureMotion(key.Map, key.InstanceID, key.GUID); motion != nil {
					dmg = s.server.fireCreatureDamageTaken(ctx, motion, attacker, dmg)
				}
			}
		}

		if dmg >= targetHealth {
			// Target slain by DoT
			if s.server != nil {
				s.server.motionMu.Lock()
				motion := s.server.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
				if motion != nil {
					s.server.clearInstanceEncounter(motion)
					motion.Health = 0
					motion.DynamicFlags |= unitDynFlagLootable
					motion.InCombat = false
					motion.TargetGUID = 0
					motion.Moving = false
					if motion.ThreatMgr != nil {
						motion.ThreatMgr.ClearThreat()
					}
				}
				s.server.motionMu.Unlock()

				s.server.stopCreatureMotionInInstance(target.Map, target.InstanceID, target.GUID, target.X, target.Y, target.Z)
				s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{
					unitFieldHealth:       0,
					unitFieldDynamicFlags: 1, // UNIT_DYNFLAG_LOOTABLE
				})
				s.server.broadcastThreatClearInInstance(target.Map, target.InstanceID, target.GUID)
				s.server.clearCreatureAuras(key)
			}
			_ = s.sendAttackStop(target.GUID, true)
			s.attackTarget = 0
			// Eluna CREATURE_EVENT_ON_TARGET_DIED (3) attacker: the periodic
			// tick's killer is the aura caster — a pet motion when the DoT
			// came from a pet (pet_combat.go), else the player (nil).
			var killer *creatureMotion
			if s.server != nil && aura.CasterGUID != 0 && (s.player == nil || aura.CasterGUID != s.playerGUID) {
				if pm := s.server.findCreatureMotion(target.Map, target.InstanceID, aura.CasterGUID); pm != nil && pm.Health > 0 {
					killer = pm
				}
			}
			s.onCreatureKilled(ctx, target, killer)
			return false
		} else {
			newHealth := targetHealth - dmg
			if s.server != nil {
				s.server.motionMu.Lock()
				motion := s.server.findCreatureMotionLocked(key.Map, key.InstanceID, key.GUID)
				if motion != nil {
					motion.Health = newHealth
					motion.InCombat = true
					if motion.ThreatMgr == nil {
						motion.ThreatMgr = NewThreatManager(target.GUID)
					}
					dist := distance3D(s.player.X, s.player.Y, s.player.Z, motion.X, motion.Y, motion.Z)
					inMelee := dist <= meleeAttackRange
					motion.ThreatMgr.AddThreat(s.playerGUID, float32(dmg), inMelee)
					motion.Moving = true
				}
				s.server.motionMu.Unlock()
				s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{unitFieldHealth: newHealth})
				s.server.triggerCreatureAggro(ctx, target.GUID, s.playerGUID)
			}
			return true
		}

	case 8, 20: // SPELL_AURA_PERIODIC_HEAL, SPELL_AURA_OBS_MOD_HEALTH
		heal := aura.Amount
		curHP := target.Health
		maxHP := target.MaxHealth
		newHP := curHP + heal
		overheal := uint32(0)
		if newHP > maxHP {
			overheal = newHP - maxHP
			newHP = maxHP
		}

		logPkt := protocol.BuildPeriodicAuraLogHeal(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, heal, overheal, 0, false)
		_ = s.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if s.server != nil {
			s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, s)
			s.server.motionMu.Lock()
			motion := s.server.findCreatureMotionLocked(s.player.Map, s.player.InstanceID, aura.TargetGUID)
			if motion != nil {
				motion.Health = newHP
			}
			s.server.motionMu.Unlock()
			s.server.broadcastCreatureValuesUpdateInInstance(target.Map, target.InstanceID, target.GUID, map[int]uint32{unitFieldHealth: newHP})
		}
		return true

	case 23: // SPELL_AURA_PERIODIC_TRIGGER_SPELL
		if aura.TriggerSpell != 0 && s.server != nil && s.server.Data != nil {
			if trigger, found, err := s.server.Data.Spell(aura.TriggerSpell); err == nil && found {
				controlledByPlayer := aura.CasterGUID == s.playerGUID
				if !controlledByPlayer && s.server != nil {
					controlledByPlayer = s.server.findSessionByGUID(aura.CasterGUID) != nil
				}
				if damage, ok := creatureSpellDamage(s.server, trigger, uint32(aura.CasterLevel), controlledByPlayer); ok && damage > 0 {
					// C++ HandlePeriodicTriggerSpellAuraTick casts via
					// CastSpellExtraArgs(AuraEffect) = TRIGGERED_FULL_MASK, so
					// TRIGGERED_DISALLOW_PROC_EVENTS applies here too.
					s.triggeredNoProcEvents++
					s.executeSpellDamage(ctx, aura.TargetGUID, aura.TriggerSpell, damage, 0)
					s.triggeredNoProcEvents--
				}
			}
		}
		return true

	case 24: // SPELL_AURA_PERIODIC_ENERGIZE
		powerType := uint32(aura.MiscValue)
		logPkt := protocol.BuildPeriodicAuraLogEnergize(aura.TargetGUID, aura.CasterGUID, aura.SpellID, aura.AuraType, powerType, aura.Amount)
		_ = s.write(uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, true)
		if s.server != nil {
			s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_PERIODICAURALOG), logPkt, s)
		}
		s.adjustSpellPower(ctx, aura.TargetGUID, aura.MiscValue, int64(aura.Amount))
		return true
	}
	return true
}

func (s *session) expireCreatureAura(key creatureAuraKey, spellID uint32, slot uint8) {
	if s.server == nil {
		return
	}
	wasCharm := false
	charmerGUID := uint64(0)
	s.server.auraMu.Lock()
	if s.server.activeCreatureAuras != nil {
		if auras, ok := s.server.activeCreatureAuras[key]; ok {
			if aura, exists := auras[spellID]; exists && aura != nil {
				wasCharm = aura.AuraType == spellAuraCharm
				charmerGUID = aura.CasterGUID
				aura.Stopped = true
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
				delete(auras, spellID)
				s.server.unregisterSingleCastAura(aura)
			}
		}
	}
	if s.server.creatureAuras != nil {
		if auras, ok := s.server.creatureAuras[key]; ok {
			delete(auras, spellID)
		}
	}
	s.server.auraMu.Unlock()
	if wasCharm {
		s.server.uncharmCreature(key, charmerGUID)
		if charmer := s.server.findSessionByGUID(charmerGUID); charmer != nil && charmer.player != nil && charmer.player.Map == key.Map && charmer.player.InstanceID == key.InstanceID {
			charmer.sendClientControl(key.GUID, false)
			charmer.sendVehiclePetSpells(0, nil)
		}
	}

	removePkt := protocol.BuildAuraUpdate(key.GUID, s.playerGUID, slot, 0, true, false, 0, 0, 1)
	s.server.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_AURA_UPDATE), removePkt, nil)
}

func (s *Server) removeCreatureAura(key creatureAuraKey, spellID uint32) {
	if s == nil || key.GUID == 0 || spellID == 0 {
		return
	}
	wasCharm := false
	charmerGUID := uint64(0)
	s.auraMu.Lock()
	var slot uint8
	if s.activeCreatureAuras != nil {
		if auras, ok := s.activeCreatureAuras[key]; ok {
			if aura, exists := auras[spellID]; exists && aura != nil {
				wasCharm = aura.AuraType == spellAuraCharm
				charmerGUID = aura.CasterGUID
				aura.Stopped = true
				slot = aura.Slot
				if aura.Timer != nil {
					aura.Timer.Stop()
				}
				if aura.TickTimer != nil {
					aura.TickTimer.Stop()
				}
				delete(auras, spellID)
				s.unregisterSingleCastAura(aura)
			}
		}
	}
	if s.creatureAuras != nil {
		if auras, ok := s.creatureAuras[key]; ok {
			delete(auras, spellID)
		}
	}
	s.auraMu.Unlock()
	if wasCharm {
		s.uncharmCreature(key, charmerGUID)
		if charmer := s.findSessionByGUID(charmerGUID); charmer != nil && charmer.player != nil && charmer.player.Map == key.Map && charmer.player.InstanceID == key.InstanceID {
			charmer.sendClientControl(key.GUID, false)
			charmer.sendVehiclePetSpells(0, nil)
		}
	}

	removePkt := protocol.BuildAuraUpdate(key.GUID, 0, slot, 0, true, false, 0, 0, 1)
	s.broadcastToInstance(key.Map, key.InstanceID, uint16(protocol.OpcodeSMSG_AURA_UPDATE), removePkt, nil)
}

// handleCancelMountAura processes CMSG_CANCEL_MOUNT_AURA (0x375).
// Reference: WorldSession::HandleCancelMountAuraOpcode (SpellHandler.cpp:544).
func (s *session) handleCancelMountAura(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	for _, aura := range s.loadedAuras() {
		if aura != nil && aura.AuraType == spellAuraMounted {
			s.removeAura(aura.SpellID)
		}
	}
	if s.player.MountDisplayID != 0 {
		s.player.MountDisplayID = 0
		s.sendPlayerMountUpdate()
		s.sendPlayerDismount()
	}
	return true
}

// handleCancelGrowthAura processes CMSG_CANCEL_GROWTH_AURA (0x29B).
// Reference: WorldSession::HandleCancelGrowthAuraOpcode (SpellHandler.cpp:535).
func (s *session) handleCancelGrowthAura(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	if s.scale != 1.0 {
		s.scale = 1.0
		s.sendPlayerUpdate()
	}
	return true
}

// handleCancelAutoRepeatSpell processes CMSG_CANCEL_AUTO_REPEAT_SPELL (0x26D).
// Reference: WorldSession::HandleCancelAutoRepeatSpellOpcode (SpellHandler.cpp:553) and CombatPackets.h:115.
func (s *session) handleCancelAutoRepeatSpell(payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	s.autoRepeatSpell = 0
	s.autoRepeatTarget = 0
	buf := protocol.NewBuffer(9)
	buf.WritePackedGUID(s.playerGUID)
	_ = s.write(uint16(protocol.OpcodeSMSG_CANCEL_AUTO_REPEAT), buf.Bytes(), true)
	return true
}

// handleCancelTempEnchantment processes CMSG_CANCEL_TEMP_ENCHANTMENT (0x379).
// Reference: WorldSession::HandleCancelTempEnchantmentOpcode (ItemHandler.cpp:1145).
func (s *session) handleCancelTempEnchantment(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil {
		return true
	}
	r := protocol.NewReader(payload)
	slot, err := r.ReadU32()
	if err != nil {
		return false
	}
	_ = slot
	s.sendPlayerUpdate()
	return true
}

// handleCorpseMapPositionQuery processes CMSG_CORPSE_MAP_POSITION_QUERY (0x4B6).
// Reference: WorldSession::HandleCorpseMapPositionQuery (QueryHandler.cpp:317).
func (s *session) handleCorpseMapPositionQuery(payload []byte) bool {
	r := protocol.NewReader(payload)
	_, _ = r.ReadU32() // unk

	buf := protocol.NewBuffer(16)
	buf.WriteF32(0)
	buf.WriteF32(0)
	buf.WriteF32(0)
	buf.WriteF32(0)
	return s.write(uint16(protocol.OpcodeSMSG_CORPSE_MAP_POSITION_QUERY_RESPONSE), buf.Bytes(), true) == nil
}

func buildCastFailed(castID uint8, spellID uint32, result uint8) []byte {
	buf := protocol.NewBuffer(6)
	buf.WriteU8(castID)
	buf.WriteU32(spellID)
	buf.WriteU8(result)
	return buf.Bytes()
}

func (s *session) savePlayerSpellCooldowns(ctx context.Context, tx *sql.Tx, state *playerState) error {
	if s == nil || tx == nil || state == nil || s.server == nil || s.server.CharactersStore == nil {
		return nil
	}
	if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_DEL_CHAR_SPELL_COOLDOWNS", state.GUID); err != nil {
		return err
	}
	bySpell := make(map[uint32]spellCooldown, len(state.Cooldowns))
	for _, cooldown := range state.Cooldowns {
		bySpell[cooldown.Spell] = cooldown
	}
	spellIDs := make([]uint32, 0, len(bySpell))
	for spellID := range bySpell {
		spellIDs = append(spellIDs, spellID)
	}
	sort.Slice(spellIDs, func(i, j int) bool { return spellIDs[i] < spellIDs[j] })
	now := time.Now().Unix()
	for _, spellID := range spellIDs {
		cooldown := bySpell[spellID]
		if cooldown.End <= now {
			continue
		}
		if _, err := s.server.CharactersStore.ExecStatementTx(ctx, tx, "CHAR_INS_CHAR_SPELL_COOLDOWN", state.GUID, cooldown.Spell, cooldown.Item, cooldown.End, cooldown.Category, cooldown.CategoryEnd); err != nil {
			return err
		}
	}
	return nil
}

func buildSpellCooldown(playerGUID uint64, spellID uint32, cooldownDurationMs uint32) []byte {
	buf := protocol.NewBuffer(8 + 1 + 4 + 4)
	buf.WriteU64(playerGUID)
	buf.WriteU8(0) // flags = 0
	buf.WriteU32(spellID)
	buf.WriteU32(cooldownDurationMs)
	return buf.Bytes()
}

// resetCastCooldownCheat mirrors SpellHistory::ResetCooldown(spellId, true)
// fired from Spell::_cast's tail (Spell.cpp:3517-3523) when the caster holds
// CHEAT_COOLDOWN (Player.h:827): the just-cast spell's own RecoveryTime
// cooldown entry is erased and SMSG_CLEAR_COOLDOWN (spell id then caster
// guid, SpellHistory.cpp:432-451) goes to the caster. Like C++ only the
// spell's own cooldown storage entry is cleared — category cooldowns are
// untouched (ResetCooldown erases _spellCooldowns[spellId] only). The
// C++ gate is modOwner->GetCommandStatus(CHEAT_COOLDOWN) with
// m_originalCaster non-null, both vacuous here: finishSpellCast always
// runs on the casting player session. The SetSpellModTakingSpell(this,
// false) preceding it is covered by finishSpellCast's deferred
// endSpellModTaking (the _cast tail window, spellmod.go);
// SetExecutedCurrently(false) has no Go model (no cast Spell object).
func (s *session) resetCastCooldownCheat(spellID uint32) {
	if s == nil || s.player == nil || s.player.ActiveCheats&cheatCooldown == 0 {
		return
	}
	removed := false
	s.playerStateMu.Lock()
	kept := s.player.Cooldowns[:0]
	for _, cd := range s.player.Cooldowns {
		if cd.Spell == spellID {
			removed = true
			continue
		}
		kept = append(kept, cd)
	}
	s.player.Cooldowns = kept
	s.playerStateMu.Unlock()
	if !removed {
		return
	}
	buf := protocol.NewBuffer(4 + 8)
	buf.WriteU32(spellID)
	buf.WriteU64(s.playerGUID)
	_ = s.write(uint16(protocol.OpcodeSMSG_CLEAR_COOLDOWN), buf.Bytes(), true)
}

// handleFarSight processes CMSG_FAR_SIGHT (0x27A).
// Reference: WorldSession::HandleFarSightOpcode (SpellHandler.cpp).
func (s *session) handleFarSight(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	op := payload[0]
	s.debug("far sight opcode", "account", s.accountName, "op", op)
	return true
}

// handleGetMirrorImageData processes CMSG_GET_MIRRORIMAGE_DATA (0x401).
// Reference: WorldSession::HandleMirrorImageDataRequest (SpellHandler.cpp:635).
func (s *session) handleGetMirrorImageData(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()

	buf := protocol.NewBuffer(68)
	buf.WriteU64(guid)
	buf.WriteU32(0) // displayId
	buf.WriteU8(s.player.Race)
	buf.WriteU8(s.player.Gender)
	buf.WriteU8(s.player.Class)
	buf.WriteU8(s.player.Skin)
	buf.WriteU8(s.player.Face)
	buf.WriteU8(s.player.HairStyle)
	buf.WriteU8(s.player.HairColor)
	buf.WriteU8(s.player.FacialStyle)
	buf.WriteU32(0) // guildId
	for i := 0; i < 11; i++ {
		buf.WriteU32(0) // outfit item displays
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_MIRRORIMAGE_DATA), buf.Bytes(), true)
	return true
}

// handleTotemDestroyed processes CMSG_TOTEM_DESTROYED (0x413).
// Reference: WorldSession::HandleTotemDestroyed (SpellHandler.cpp:582).
func (s *session) handleTotemDestroyed(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 1 {
		return true
	}
	slotID := payload[0]
	if slotID >= 4 {
		return true
	}
	s.destroyTotem(slotID)
	s.debug("totem destroyed", "account", s.accountName, "slot", slotID)
	return true
}

// handleSpellClick processes CMSG_SPELLCLICK (0x410).
// Reference: WorldSession::HandleSpellClick (SpellHandler.cpp:616) -> Unit::HandleSpellClick (Unit.cpp:12982).
func (s *session) handleSpellClick(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	targetGUID, err := r.ReadU64()
	if err != nil || targetGUID == 0 {
		return true
	}

	npcEntry := uint32((targetGUID >> 24) & 0x00FFFFFF)
	s.debug("spell click", "account", s.accountName, "target", targetGUID, "entry", npcEntry)

	if s.server == nil || s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
		return true
	}

	rows, err := s.server.WorldStore.DB.QueryContext(ctx, "SELECT spell_id, cast_flags, user_type FROM npc_spellclick_spells WHERE npc_entry = ?", npcEntry)
	if err != nil {
		return true
	}
	defer rows.Close()

	type spellClick struct {
		spellID   uint32
		castFlags uint8
		userType  uint8
	}
	var clicks []spellClick
	for rows.Next() {
		var sp, cf, ut uint32
		if err := rows.Scan(&sp, &cf, &ut); err == nil && sp > 0 {
			clicks = append(clicks, spellClick{
				spellID:   sp,
				castFlags: uint8(cf),
				userType:  uint8(ut),
			})
		}
	}

	for _, click := range clicks {
		targetUnit := targetGUID
		if click.castFlags&0x02 != 0 { // NPC_CLICK_CAST_TARGET_CLICKER
			targetUnit = s.playerGUID
		}

		if s.server.Data != nil {
			if spell, found, err := s.server.Data.Spell(click.spellID); err == nil && found {
				targetData := protocol.SpellTargetData{
					Flags:    protocol.SpellTargetFlagUnitWireMask,
					UnitGUID: targetUnit,
				}
				s.finishSpellCast(ctx, 0, click.spellID, spell, targetData, 0, 0)
			}
		}
	}
	return true
}

// handleTalentWipeConfirm processes MSG_TALENT_WIPE_CONFIRM (0x2AA).
// Reference: WorldSession::HandleTalentWipeConfirmOpcode (SpellHandler.cpp:732).
func (s *session) handleTalentWipeConfirm(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 8 {
		return true
	}
	r := protocol.NewReader(payload)
	wipeGUID, _ := r.ReadU64()

	// Clear player talents and unlearn all talent spells
	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		rows, err := cdb.QueryContext(ctx, "SELECT spell FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, s.player.ActiveTalentGroup)
		if err == nil {
			var unlearnSpells []uint32
			for rows.Next() {
				var sp int64
				if rows.Scan(&sp) == nil && sp > 0 {
					unlearnSpells = append(unlearnSpells, uint32(sp))
				}
			}
			rows.Close()
			for _, sp := range unlearnSpells {
				_, _ = cdb.ExecContext(ctx, "DELETE FROM character_spell WHERE guid = ? AND spell = ?", s.playerGUID, sp)
				if s.hasAura(sp) {
					s.removeAura(sp)
				}
				s.removeOwnerPetAurasForSpell(ctx, sp)
				unlearnBuf := protocol.NewBuffer(4)
				unlearnBuf.WriteU32(sp)
				_ = s.write(uint16(protocol.OpcodeSMSG_REMOVED_SPELL), unlearnBuf.Bytes(), true)
			}
		}
		_, _ = cdb.ExecContext(ctx, "DELETE FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, s.player.ActiveTalentGroup)
	}
	s.player.Talents = make(map[uint32]uint8)

	buf := protocol.NewBuffer(12)
	buf.WriteU64(wipeGUID)
	buf.WriteU32(0) // free or cost
	_ = s.write(uint16(protocol.OpcodeMSG_TALENT_WIPE_CONFIRM), buf.Bytes(), true)

	// Cast visual untalent effect 14867 from trainer to player
	castPkt := protocol.NewBuffer(16)
	castPkt.WritePackedGUID(wipeGUID)
	castPkt.WritePackedGUID(s.playerGUID)
	castPkt.WriteU8(1)
	castPkt.WriteU32(14867)
	castPkt.WriteU32(0)
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_GO), castPkt.Bytes(), true)
	_ = s.sendTalentsInfo(false)
	s.sendPlayerUpdate()
	return true
}

const PlayerExtraHas310Flyer uint32 = 0x0040

type LearnedMountSpell struct {
	ID                 uint32
	MountedFlightSpeed int
}

type MountState struct {
	extraFlags uint32
	spells     map[uint32]LearnedMountSpell
}

func NewMountState(extraFlags uint32, spells []LearnedMountSpell) *MountState {
	state := &MountState{extraFlags: extraFlags, spells: make(map[uint32]LearnedMountSpell, len(spells))}
	for _, spell := range spells {
		state.spells[spell.ID] = spell
	}
	return state
}

func (s *MountState) ExtraFlags() uint32 {
	return s.extraFlags
}

func (s *MountState) Has310Flyer(checkAllSpells bool, excludeSpellID uint32) bool {
	if !checkAllSpells {
		return s.extraFlags&PlayerExtraHas310Flyer != 0
	}
	s.extraFlags &^= PlayerExtraHas310Flyer
	for _, spell := range s.spells {
		if spell.ID != excludeSpellID && spell.MountedFlightSpeed == 310 {
			s.extraFlags |= PlayerExtraHas310Flyer
			return true
		}
	}
	return false
}

func (s *MountState) SetHas310Flyer(enabled bool) {
	if enabled {
		s.extraFlags |= PlayerExtraHas310Flyer
	} else {
		s.extraFlags &^= PlayerExtraHas310Flyer
	}
}

func (s *MountState) LearnSpell(spell LearnedMountSpell) {
	s.spells[spell.ID] = spell
	if spell.MountedFlightSpeed == 310 {
		s.SetHas310Flyer(true)
	}
}

func (s *MountState) UnlearnSpell(id uint32) bool {
	if _, ok := s.spells[id]; !ok {
		return s.Has310Flyer(false, 0)
	}
	delete(s.spells, id)
	return s.Has310Flyer(true, 0)
}

func (s *MountState) PreferredFlightSpeed(canFly bool) int {
	if !canFly {
		return 0
	}
	if s.Has310Flyer(false, 0) {
		return 310
	}
	return 280
}

// handleRemoveGlyph processes CMSG_REMOVE_GLYPH (0x48A).
// Reference: WorldSession::HandleRemoveGlyph (SpellHandler.cpp:840): read the
// slot index, and when a glyph is socketed there clear it, persist the change,
// and resend the talent panel so the client drops the glyph. The reference
// also removes the glyph's granted aura; the Go server has no aura engine yet.
func (s *session) handleRemoveGlyph(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 4 {
		return true
	}
	r := protocol.NewReader(payload)
	slot, err := r.ReadU32()
	if err != nil {
		return false
	}
	if slot >= 6 {
		return true
	}
	spec := s.player.ActiveTalentGroup
	if spec >= 2 {
		spec = 0
	}
	s.player.Glyphs[spec][slot] = 0

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		col := fmt.Sprintf("glyph%d", slot+1)
		_, _ = cdb.ExecContext(ctx, fmt.Sprintf("UPDATE character_glyphs SET %s = 0 WHERE guid = ? AND talentGroup = ?", col), s.playerGUID, spec)
	}
	_ = s.sendTalentsInfo(false)
	return true
}

// applyGlyph sockets a glyph into the specified slot for the active talent group.
// Reference: Spell::EffectApplyGlyph (SpellEffects.cpp:4018).
func (s *session) applyGlyph(ctx context.Context, slot uint8, glyphPropID uint16) {
	if s.player == nil || slot >= 6 || glyphPropID == 0 {
		return
	}
	spec := s.player.ActiveTalentGroup
	if spec >= 2 {
		spec = 0
	}
	s.player.Glyphs[spec][slot] = glyphPropID

	if s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		cdb := s.server.CharactersStore.DB
		col := fmt.Sprintf("glyph%d", slot+1)
		_, _ = cdb.ExecContext(ctx, "INSERT OR IGNORE INTO character_glyphs (guid, talentGroup, glyph1, glyph2, glyph3, glyph4, glyph5, glyph6) VALUES (?, ?, 0, 0, 0, 0, 0, 0)", s.playerGUID, spec)
		_, _ = cdb.ExecContext(ctx, fmt.Sprintf("UPDATE character_glyphs SET %s = ? WHERE guid = ? AND talentGroup = ?", col), glyphPropID, s.playerGUID, spec)
	}
	_ = s.sendTalentsInfo(false)
}

// handleUpdateMissileTrajectory processes CMSG_UPDATE_MISSILE_TRAJECTORY (0x462).
// Reference: WorldSession::HandleUpdateMissileTrajectory (MiscHandler.cpp:1545).
func (s *session) handleUpdateMissileTrajectory(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 45 {
		return true
	}
	r := protocol.NewReader(payload)
	guid, _ := r.ReadU64()
	spellID, _ := r.ReadU32()
	elevation, _ := r.ReadF32()
	speed, _ := r.ReadF32()
	fireX, _ := r.ReadF32()
	fireY, _ := r.ReadF32()
	fireZ, _ := r.ReadF32()
	impactX, _ := r.ReadF32()
	impactY, _ := r.ReadF32()
	impactZ, _ := r.ReadF32()
	moveStop, _ := r.ReadU8()

	s.debug("update missile trajectory", "account", s.accountName, "guid", guid, "spell", spellID, "elevation", elevation, "speed", speed, "fire", []float32{fireX, fireY, fireZ}, "impact", []float32{impactX, impactY, impactZ}, "moveStop", moveStop)

	if moveStop != 0 && r.Remaining() >= 4 {
		opcode, _ := r.ReadU32()
		s.handleMovement(ctx, opcode, payload[r.Position():])
	}
	return true
}

// handleUpdateProjectilePosition processes CMSG_UPDATE_PROJECTILE_POSITION (0x4BE).
// Reference: WorldSession::HandleUpdateProjectilePosition (SpellHandler.cpp:816).
func (s *session) handleUpdateProjectilePosition(ctx context.Context, payload []byte) bool {
	if !s.playerLoaded || s.player == nil || len(payload) < 25 {
		return true
	}
	r := protocol.NewReader(payload)
	casterGuid, _ := r.ReadU64()
	spellID, _ := r.ReadU32()
	castCount, _ := r.ReadU8()
	hitX, _ := r.ReadF32()
	hitY, _ := r.ReadF32()
	hitZ, _ := r.ReadF32()

	s.debug("update projectile position", "account", s.accountName, "guid", casterGuid, "spell", spellID, "castCount", castCount, "hit", []float32{hitX, hitY, hitZ})
	return true
}

func (s *session) sendActionButtons() {
	if s.player == nil {
		return
	}
	_ = s.write(uint16(protocol.OpcodeSMSG_ACTION_BUTTONS), buildActionButtons(s.player.Actions), true)
}

// activateSpec mirrors Player::ActivateSpec (Player.cpp:26292).
func (s *session) activateSpec(ctx context.Context, targetSpec uint8) {
	if s.player == nil || targetSpec >= s.player.TalentGroupsCount || targetSpec == s.player.ActiveTalentGroup {
		return
	}
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return
	}
	cdb := s.server.CharactersStore.DB

	// 1. Unlearn talents of current spec
	rows, err := cdb.QueryContext(ctx, "SELECT spell FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, s.player.ActiveTalentGroup)
	if err == nil {
		var oldSpells []uint32
		for rows.Next() {
			var sp int64
			if rows.Scan(&sp) == nil && sp > 0 {
				oldSpells = append(oldSpells, uint32(sp))
			}
		}
		rows.Close()
		for _, sp := range oldSpells {
			_, _ = cdb.ExecContext(ctx, "DELETE FROM character_spell WHERE guid = ? AND spell = ?", s.playerGUID, sp)
			if s.hasAura(sp) {
				s.removeAura(sp)
			}
			s.removeOwnerPetAurasForSpell(ctx, sp)
			unlearnBuf := protocol.NewBuffer(4)
			unlearnBuf.WriteU32(sp)
			_ = s.write(uint16(protocol.OpcodeSMSG_REMOVED_SPELL), unlearnBuf.Bytes(), true)
		}
	}

	// 2. Set new active talent group
	s.player.ActiveTalentGroup = targetSpec
	_, _ = cdb.ExecContext(ctx, "UPDATE characters SET activeTalentGroup = ? WHERE guid = ?", targetSpec, s.playerGUID)

	// 3. Load talents for new spec
	s.loadPlayerTalents(ctx, s.player)

	// 4. Teach spells for new spec
	var newSpells []uint32
	tRows, err := cdb.QueryContext(ctx, "SELECT spell FROM character_talent WHERE guid = ? AND talentGroup = ?", s.playerGUID, targetSpec)
	if err == nil {
		for tRows.Next() {
			var sp int64
			if tRows.Scan(&sp) == nil && sp > 0 {
				newSpells = append(newSpells, uint32(sp))
			}
		}
		tRows.Close()
		for _, sp := range newSpells {
			_, _ = cdb.ExecContext(ctx, "REPLACE INTO character_spell (guid, spell, active, disabled) VALUES (?, ?, 1, 0)", s.playerGUID, sp)
			if !hasLearnedSpell(s.player.Spells, sp) {
				s.player.Spells = append(s.player.Spells, learnedSpell{ID: sp, Active: true})
			}
			learnBuf := protocol.NewBuffer(6)
			learnBuf.WriteU32(sp)
			learnBuf.WriteU16(0)
			_ = s.write(uint16(protocol.OpcodeSMSG_LEARNED_SPELL), learnBuf.Bytes(), true)
		}
	}

	// 5. Load and send action buttons for new spec
	if actions, err := s.loadActionButtons(ctx, s.playerGUID, s.player.Spells); err == nil {
		s.player.Actions = actions
		s.sendActionButtons()
	}

	// 6. Send talents info update
	_ = s.sendTalentsInfo(false)
}

// Channel and pushback state machine, mirroring the reference:
//   - Spell::handle_immediate / SendChannelStart (Spell.cpp:3568/4661):
//     channeled spells (SPELL_ATTR1_CHANNELED_1 0x04 | CHANNELED_2 0x40 in
//     AttributesEx field 6) start a channel after SPELL_GO, announced with
//     MSG_CHANNEL_START (packed caster GUID, spell id, duration) and periodic
//     effect ticks every EffectAuraPeriod milliseconds.
//   - Spell::Delayed (Spell.cpp:7250): a player taking damage while casting a
//     spell with SPELL_INTERRUPT_FLAG_PUSH_BACK 0x02 loses 500ms per hit, at
//     most twice per cast, clamped to the remaining cast time, announced via
//     SMSG_SPELL_DELAYED (packed caster GUID, delay).
//   - Spell::DelayedChannel (Spell.cpp:7290): a channeling player taking
//     damage on a spell with CHANNEL_FLAG_DELAY 0x4000 loses 25% of the total
//     channel duration per hit, at most twice, announced via MSG_CHANNEL_UPDATE.
//   - Movement interrupts: movement flags cancel the active cast and channel
//     (movement.go), matching reference movement-cast interruption.

const (
	spellAttr1Channeled1     uint32 = 0x04
	spellAttr1Channeled2     uint32 = 0x40
	spellInterruptPushBack   uint32 = 0x02
	spellInterruptAbortOnDmg uint32 = 0x10
	channelFlagDelay         uint32 = 0x4000
	maxSpellPushbacks        int    = 2
	defaultCastPushbackMs    uint32 = 500
)

// activeChannelState tracks one running channeled spell.
type activeChannelState struct {
	CastID     uint8
	SpellID    uint32
	TargetGUID uint64
	TargetKey  creatureAuraKey
	Spell      wotlk.Spell
	DurationMs uint32
	Remaining  time.Duration
	PeriodMs   uint32
	Pushbacks  int
	Timer      *time.Timer
	TickTimer  *time.Timer
	DrainTimer *time.Timer
	Stopped    bool
	// HasDest/DestX/DestY/DestZ record the channeled spell's destination
	// (C++ SpellCastTargets::HasDst on the channeled Spell::m_targets).
	// Spell::SelectImplicitChannelTargets reads it for
	// TARGET_DEST_CHANNEL_TARGET (Spell.cpp:1010).
	HasDest bool
	DestX   float32
	DestY   float32
	DestZ   float32
}

func isChanneledSpell(spell wotlk.Spell) bool {
	return spell.AttributesEx&(spellAttr1Channeled1|spellAttr1Channeled2) != 0
}

// isRangedWeaponSpell mirrors SpellInfo::IsRangedWeaponSpell
// (SpellInfo.cpp:1249-1254): the hunter-family arm (minus the 53352
// flag-1 carve-out), the ranged-subclass mask on the raw DBC
// EquippedItemSubClass field, or SPELL_ATTR0_REQ_AMMO.
func isRangedWeaponSpell(spell wotlk.Spell) bool {
	return (spell.SpellFamilyName == spellFamilyHunter && spell.SpellFamilyFlags[1]&0x10000000 == 0) ||
		spell.EquippedItemSubClass&itemSubclassMaskWeaponRanged != 0 ||
		spell.Attributes&spellAttr0ReqAmmo != 0
}

// spellAllowsDeadTarget mirrors SpellInfo::IsAllowingDeadTarget (SpellInfo.cpp:1177):
// ATTR2_CAN_TARGET_DEAD or a corpse/dead-unit bit in the DBC Targets mask (Spell.dbc field 16).
func spellAllowsDeadTarget(spell wotlk.Spell) bool {
	return spell.AttributesEx1&spellAttr2CanTargetDead != 0 ||
		spell.Targets&(targetFlagCorpseEnemy|targetFlagUnitDead|targetFlagCorpseAlly) != 0
}

// sendChannelUpdate mirrors Spell::SendChannelUpdate: packed caster GUID plus
// remaining channel milliseconds; zero clears the client channel bar.
func (s *session) sendChannelUpdate(remainingMs uint32) {
	packet := protocol.NewBuffer(12)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(remainingMs)
	_ = s.write(uint16(protocol.OpcodeMSG_CHANNEL_UPDATE), packet.Bytes(), true)
	if remainingMs == 0 {
		s.sendPlayerUpdate()
	}
}

// startChannel begins the channeled phase of a finished cast: broadcast the
// channel start, schedule periodic ticks, and arm completion. hasDest/dest*
// record the cast's destination (SpellCastTargets::HasDst analog) so
// SelectImplicitChannelTargets parity (Spell.cpp:1010) can resolve
// TARGET_DEST_CHANNEL_TARGET for spells triggered during the channel.
func (s *session) startChannel(castID uint8, spellID uint32, spell wotlk.Spell, targetGUID uint64, hasDest bool, destX, destY, destZ float32) {
	if s.player == nil || s.server.Data == nil {
		return
	}
	var durationMs int32 = 0
	if value, ok, err := s.server.Data.SpellDurationBase(spell.DurationIndex); err == nil && ok {
		durationMs = value
	}
	if durationMs == -1 {
		// Spell::handle_immediate (Spell.cpp:3582-3583): infinite channels
		// (GetDuration() == -1) SendChannelStart(-1) and enter
		// SPELL_STATE_CASTING until interrupted — no completion timer. Go
		// has no infinite-channel state; the cast falls through without
		// starting one (no channel bar, no per-tick drain). Noted, not bridged.
		return
	}
	if durationMs <= 0 {
		return // instant channels have no timed lifecycle here
	}
	// Spell::handle_immediate (Spell.cpp:3577-3585): "First mod_duration
	// then haste - see Missile Barrage" — SPELLMOD_DURATION folds flat/pct
	// duration mods (talents, glyphs) before the haste compression.
	durationMs = s.applySpellMod(spell, spellModDuration, durationMs)
	period := uint32(0)
	for _, effect := range spell.Effects {
		if effect.Effect != 0 && effect.AuraPeriod > period {
			period = effect.AuraPeriod
		}
	}

	// In WotLK 3.3.5, channeled spells scale with spell haste: duration and tick interval are compressed
	// Mirrors Unit::ModSpellDurationTime via Spell::handle_immediate (Spell.cpp:3580-3585):
	// the modded duration compresses by (1 + haste), matching the C++ order (mod first, then haste).
	hastePct := s.getSpellHastePct()
	if hastePct > 0 {
		durationMs = int32(math.Round(float64(durationMs) / (1.0 + hastePct/100.0)))
		if period > 0 {
			period = uint32(math.Round(float64(period) / (1.0 + hastePct/100.0)))
		}
	}
	// Spell::handle_immediate (Spell.cpp:3588-3591): channeled spells with
	// nonzero duration take SPELL_STATE_CASTING and AddInterruptMask
	// (ChannelInterruptFlags). Go has no interrupt-mask model: movement
	// cancels the active channel unconditionally (movement.go), so channels
	// that C++ would let move (IsMoveAllowedChannel) are also stopped here.
	// Noted, not bridged.
	channel := &activeChannelState{
		CastID:     castID,
		SpellID:    spellID,
		TargetGUID: targetGUID,
		TargetKey:  creatureAuraKeyForPlayer(*s.player, targetGUID),
		Spell:      spell,
		DurationMs: uint32(durationMs),
		Remaining:  time.Duration(durationMs) * time.Millisecond,
		PeriodMs:   period,
		HasDest:    hasDest,
		DestX:      destX,
		DestY:      destY,
		DestZ:      destZ,
	}
	s.castMu.Lock()
	s.activeChannel = channel
	s.castMu.Unlock()

	packet := protocol.NewBuffer(16)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(spellID)
	packet.WriteU32(uint32(durationMs))
	_ = s.write(uint16(protocol.OpcodeMSG_CHANNEL_START), packet.Bytes(), true)
	if s.server != nil {
		s.server.broadcastToNearby(uint16(protocol.OpcodeMSG_CHANNEL_START), packet.Bytes(), s)
	}
	s.sendPlayerUpdate()

	channel.Timer = time.AfterFunc(channel.Remaining, func() { s.finishChannel() })
	if period > 0 && period <= uint32(durationMs) {
		channel.TickTimer = time.AfterFunc(time.Duration(period)*time.Millisecond, func() { s.channelTick() })
	}
	// SpellAuras.cpp:436: an aura with ManaPerSecond or ManaPerSecondPerLevel
	// drains the caster every second.
	if spell.ManaPerSecond != 0 || spell.ManaPerSecondPerLevel != 0 {
		channel.DrainTimer = time.AfterFunc(time.Second, func() { s.channelDrainTick() })
	}
	s.debug("channel started", "account", s.accountName, "spell", spellID, "duration_ms", durationMs, "period_ms", period)
}

// finishChannel completes the channel: clear state and zero the bar.
// Spell::update (Spell.cpp:3874-3876) fires the creature AI
// OnSpellCastFinished(CHANNELING_COMPLETE) hook on natural completion;
// Go has no creature casters and no creature AI, so there is no hook to
// call here.
func (s *session) finishChannel() {
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	s.activeChannel = nil
	if channel.Timer != nil {
		channel.Timer.Stop()
	}
	if channel.TickTimer != nil {
		channel.TickTimer.Stop()
	}
	if channel.DrainTimer != nil {
		channel.DrainTimer.Stop()
	}
	channel.Stopped = true
	spellID := channel.SpellID
	s.castMu.Unlock()

	s.sendChannelUpdate(0)
	s.debug("channel finished", "account", s.accountName, "spell", spellID)
}

// expireChannelAuras removes the caster-owned auras of the channel's spell
// from the caster and the channel target: the Go mirror of the
// RemoveOwnedAura(spellId, m_originalCasterGUID, 0, AURA_REMOVE_BY_CANCEL)
// sweep Spell::update runs when a channeled spell ends early for lack of
// alive targets (Spell.cpp:3856-3860).
func (s *session) expireChannelAuras(channel *activeChannelState) {
	s.expirePlayerAura(channel.SpellID)
	if s.server != nil && s.player != nil && channel.TargetGUID != 0 {
		if target := s.server.findSessionByGUID(channel.TargetGUID); target != nil {
			if target != s {
				target.expirePlayerAura(channel.SpellID)
			}
		} else {
			key := channel.TargetKey
			if key.GUID == 0 {
				key = creatureAuraKeyForPlayer(*s.player, channel.TargetGUID)
			}
			s.server.auraMu.Lock()
			_, hasAura := s.server.activeCreatureAuras[key][channel.SpellID]
			s.server.auraMu.Unlock()
			if hasAura {
				s.server.removeCreatureAura(key, channel.SpellID)
			}
		}
	}
}

// interruptCurrentChannel stops the active channel without the completion
// path (movement, new cast, cancel).
func (s *session) interruptCurrentChannel() {
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	s.activeChannel = nil
	if channel.Timer != nil {
		channel.Timer.Stop()
	}
	if channel.TickTimer != nil {
		channel.TickTimer.Stop()
	}
	if channel.DrainTimer != nil {
		channel.DrainTimer.Stop()
	}
	channel.Stopped = true
	s.castMu.Unlock()

	s.expireChannelAuras(channel)
	s.sendChannelUpdate(0)
}

// channelTargetAlive answers Spell::update's UpdateChanneledTargetList
// (Spell.cpp:3853): the channel's explicit unit target is still valid while
// alive. Player targets use the live session (ghost included); creature
// targets resolve through the combat-target pipeline, which returns
// Health == 0 for dead creatures and ok == false for unresolvable GUIDs.
// A zero target (dest-only channels) has no unit to recheck.
func (s *session) channelTargetAlive(ctx context.Context, targetGUID uint64) bool {
	if targetGUID == 0 {
		return true
	}
	if s.server != nil {
		if sess := s.server.findSessionByGUID(targetGUID); sess != nil && sess.player != nil {
			return !sess.isDeadOrGhost()
		}
	}
	target, ok := s.getCombatTarget(ctx, targetGUID)
	return ok && target.Health > 0
}

// channelTick applies one periodic effect tick of the channeled spell and
// schedules the next while the channel is alive.
func (s *session) channelTick() {
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	next := time.Duration(channel.PeriodMs) * time.Millisecond
	spell := channel.Spell
	targetGUID := channel.TargetGUID
	remaining := channel.Remaining
	s.castMu.Unlock()

	ctx := context.Background()
	// Spell::update (Spell.cpp:3851-3879): the SPELL_STATE_CASTING leg
	// revalidates the channeled target list on every server tick; when no
	// alive targets remain the channel ends immediately and the caster's
	// applied auras are removed (AURA_REMOVE_BY_CANCEL), then the channel
	// completes normally (SendChannelUpdate(0) + finish(), no cast-failure
	// result and no interrupt broadcast). Go has no per-tick update loop,
	// so the check rides the period tick instead of the 50ms server tick.
	if targetGUID != 0 && !s.channelTargetAlive(ctx, targetGUID) {
		s.expireChannelAuras(channel)
		s.finishChannel()
		return
	}
	for effectIndex, effect := range spell.Effects {
		if effect.Effect == 0 || effect.Effect == 6 && effect.Aura == 23 {
			continue
		}
		amount := uint32(effect.BasePoints + 1)
		if amount == 0 {
			continue
		}
		switch effect.Effect {
		case 2, 87, 108, 17: // damage effects tick on the target
			if targetGUID != 0 && targetGUID != s.playerGUID {
				s.executeSpellDamage(ctx, targetGUID, spell.ID, amount, effectIndex)
			}
		case 6, 10, 136, 105: // auras and heals tick on the target or caster
			if targetGUID != 0 && targetGUID != s.playerGUID {
				s.executeSpellDamage(ctx, targetGUID, spell.ID, amount, effectIndex)
			} else {
				s.executeSpellHeal(ctx, s.playerGUID, spell.ID, amount, effectIndex)
			}
		}
	}

	s.castMu.Lock()
	channel = s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	remaining -= next
	if remaining > 0 {
		channel.TickTimer = time.AfterFunc(next, func() { s.channelTick() })
		s.castMu.Unlock()
		return
	}
	s.castMu.Unlock()
}

// channelDrainTick applies one second of the channeled spell's mana drain and
// re-arms while the channel lives. Mirrors Aura::Update (SpellAuras.cpp:844):
// manaPerSecond = ManaPerSecond + ManaPerSecondPerLevel * caster level;
// POWER_HEALTH drains health (needs strictly more than the drain), other
// powers need at least the drain, and a shortfall removes the aura (ends the
// channel here).
func (s *session) channelDrainTick() {
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped {
		s.castMu.Unlock()
		return
	}
	spell := channel.Spell
	s.castMu.Unlock()

	if s.player == nil {
		return
	}
	manaPerSecond := spell.ManaPerSecond + spell.ManaPerSecondPerLevel*uint32(s.player.Level)
	if manaPerSecond > 0 {
		if spell.PowerType == 0xFFFFFFFE { // POWER_HEALTH = -2 in C++
			if s.player.Health > manaPerSecond {
				s.player.Health -= manaPerSecond
				s.sendPlayerUpdate()
			} else {
				s.interruptCurrentChannel()
				return
			}
		} else if spell.PowerType < 7 {
			if s.player.Powers[spell.PowerType] >= manaPerSecond {
				s.adjustSpellPower(context.Background(), s.playerGUID, int32(spell.PowerType), -int64(manaPerSecond))
			} else {
				s.interruptCurrentChannel()
				return
			}
		}
	}

	s.castMu.Lock()
	channel = s.activeChannel
	if channel != nil && !channel.Stopped {
		channel.DrainTimer = time.AfterFunc(time.Second, func() { s.channelDrainTick() })
	}
	s.castMu.Unlock()
}

// getPushbackReductionLocked returns total percent pushback reduction from active auras (SPELL_AURA_REDUCE_PUSHBACK = 149).
// Assumes s.castMu is held.
// Reference: TrinityCore Spell::Delayed / Spell::DelayedChannel: delayReduce += playerCaster->GetTotalAuraModifier(SPELL_AURA_REDUCE_PUSHBACK) - 100.
func (s *session) getPushbackReductionLocked() int32 {
	if s == nil {
		return 0
	}
	var reduction int32
	for _, a := range s.activeAuras {
		if a != nil && !a.Stopped && a.AuraType == 149 { // SPELL_AURA_REDUCE_PUSHBACK
			reduction += int32(a.Amount)
		}
	}
	if reduction > 100 {
		reduction = 100
	}
	return reduction
}

func (s *session) getPushbackReduction() int32 {
	if s == nil {
		return 0
	}
	s.castMu.Lock()
	defer s.castMu.Unlock()
	return s.getPushbackReductionLocked()
}

// delayCurrentCast mirrors Spell::Delayed: called when the player takes
// damage during a timed cast. Requires SPELL_INTERRUPT_FLAG_PUSH_BACK, at
// most two pushbacks per cast, 500ms each clamped to remaining time, and
// announces SMSG_SPELL_DELAYED. Spells with SPELL_INTERRUPT_FLAG_ABORT_ON_DMG
// are aborted entirely on direct damage.
func (s *session) delayCurrentCast() {
	if s.player == nil {
		return
	}
	s.castMu.Lock()
	cast := s.activeCast
	if cast == nil || cast.CastTimeMs == 0 {
		s.castMu.Unlock()
		return
	}
	// Direct damage completely aborts spells with SPELL_INTERRUPT_FLAG_ABORT_ON_DMG (0x10).
	// Reference: TrinityCore Unit.cpp:944-945.
	if cast.InterruptFlg&spellInterruptAbortOnDmg != 0 {
		s.castMu.Unlock()
		s.interruptCurrentCast()
		return
	}
	if cast.InterruptFlg&spellInterruptPushBack == 0 || cast.Pushbacks >= maxSpellPushbacks {
		s.castMu.Unlock()
		return
	}
	reduction := s.getPushbackReductionLocked()
	if reduction >= 100 {
		s.castMu.Unlock()
		return
	}
	elapsed := time.Since(cast.StartAt)
	remaining := time.Duration(cast.CastTimeMs)*time.Millisecond - elapsed
	if remaining <= 0 {
		s.castMu.Unlock()
		return
	}
	delayMs := defaultCastPushbackMs
	if reduction > 0 {
		delayMs = uint32(float64(delayMs) * float64(100-reduction) / 100.0)
	}
	delay := time.Duration(delayMs) * time.Millisecond
	if delay > remaining {
		delay = remaining
	}
	cast.Pushbacks++
	newRemaining := remaining + delay
	cast.StartAt = time.Now().Add(-(time.Duration(cast.CastTimeMs)*time.Millisecond - newRemaining))
	if cast.Timer != nil {
		cast.Timer.Reset(newRemaining)
	}
	pushbacks := cast.Pushbacks
	spellID := cast.SpellID
	s.castMu.Unlock()

	packet := protocol.NewBuffer(8)
	packet.WritePackedGUID(s.playerGUID)
	packet.WriteU32(uint32(delay.Milliseconds()))
	_ = s.write(uint16(protocol.OpcodeSMSG_SPELL_DELAYED), packet.Bytes(), true)
	s.debug("cast pushed back", "account", s.accountName, "spell", spellID, "delay_ms", delay.Milliseconds(), "count", pushbacks)
}

// delayCurrentChannel mirrors Spell::DelayedChannel: called when the player
// takes damage while channeling. Requires CHANNEL_FLAG_DELAY, at most two
// pushbacks, 25% of the total channel duration per hit, announced via
// MSG_CHANNEL_UPDATE with the new remaining time.
func (s *session) delayCurrentChannel() {
	if s.player == nil {
		return
	}
	s.castMu.Lock()
	channel := s.activeChannel
	if channel == nil || channel.Stopped || channel.Spell.ChannelInterrupt&channelFlagDelay == 0 || channel.Pushbacks >= maxSpellPushbacks {
		s.castMu.Unlock()
		return
	}
	reduction := s.getPushbackReductionLocked()
	if reduction >= 100 {
		s.castMu.Unlock()
		return
	}
	delayMs := channel.DurationMs / 4 // 25% of total duration per hit
	if reduction > 0 {
		delayMs = uint32(float64(delayMs) * float64(100-reduction) / 100.0)
	}
	if delayMs == 0 {
		s.castMu.Unlock()
		return
	}
	if time.Duration(delayMs)*time.Millisecond >= channel.Remaining {
		delayMs = uint32(channel.Remaining.Milliseconds())
		channel.Remaining = 0
	} else {
		channel.Remaining -= time.Duration(delayMs) * time.Millisecond
	}
	channel.Pushbacks++
	remaining := channel.Remaining
	if channel.Timer != nil {
		channel.Timer.Reset(remaining)
	}
	spellID := channel.SpellID
	s.castMu.Unlock()

	s.sendChannelUpdate(uint32(remaining.Milliseconds()))
	s.debug("channel pushed back", "account", s.accountName, "spell", spellID, "delay_ms", delayMs, "count", channel.Pushbacks)
}

type itemTemplateClassInfo struct {
	Class    uint32
	SubClass uint32
	InvType  uint32
}

func (s *Server) getItemTemplateClassInfo(ctx context.Context, entry uint32) (itemTemplateClassInfo, bool) {
	if entry == 0 {
		return itemTemplateClassInfo{}, false
	}
	s.itemTemplateMu.RLock()
	if s.itemTemplates != nil {
		if info, ok := s.itemTemplates[entry]; ok {
			s.itemTemplateMu.RUnlock()
			return info, true
		}
	}
	s.itemTemplateMu.RUnlock()

	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return itemTemplateClassInfo{}, false
	}

	var class, subclass, invType uint32
	err := s.WorldStore.DB.QueryRowContext(ctx, "SELECT class, subclass, InventoryType FROM item_template WHERE entry = ? LIMIT 1", entry).Scan(&class, &subclass, &invType)
	if err != nil {
		return itemTemplateClassInfo{}, false
	}

	info := itemTemplateClassInfo{Class: class, SubClass: subclass, InvType: invType}
	s.itemTemplateMu.Lock()
	if s.itemTemplates == nil {
		s.itemTemplates = make(map[uint32]itemTemplateClassInfo)
	}
	s.itemTemplates[entry] = info
	s.itemTemplateMu.Unlock()
	return info, true
}

func (s *session) getItemTemplateClassInfo(ctx context.Context, entry uint32) (itemTemplateClassInfo, bool) {
	if s == nil || s.server == nil {
		return itemTemplateClassInfo{}, false
	}
	return s.server.getItemTemplateClassInfo(ctx, entry)
}

// isItemFitToSpell verifies if an item's class, subclass, and inventory type satisfy the spell's requirements.
// Reference: TrinityCore Item::IsFitToSpellRequirements (Item.cpp:799-832).
func isItemFitToSpell(spell wotlk.Spell, class uint32, subclass uint32, invType uint32) bool {
	// isEnchantSpell (Item.cpp:803): enchant spells accept vellum items for
	// armor/weapon requirements and plain weapons for mainhand/offhand
	// inventory-type requirements.
	isEnchant := spellHasEffect(spell, spellEffectEnchantItem) ||
		spellHasEffect(spell, spellEffectEnchantItemTemporary) ||
		spellHasEffect(spell, spellEffectEnchantItemPrismatic)
	if spell.EquippedItemClass >= 0 {
		if isEnchant && ((spell.EquippedItemClass == itemClassArmor && class == itemClassTradeGoods && subclass == itemSubclassArmorEnchantment) ||
			(spell.EquippedItemClass == itemClassWeapon && class == itemClassTradeGoods && subclass == itemSubclassWeaponEnchantment)) {
			return true
		}
		if uint32(spell.EquippedItemClass) != class {
			return false
		}
		if spell.EquippedItemSubClass != 0 {
			if (spell.EquippedItemSubClass & (1 << subclass)) == 0 {
				return false
			}
		}
	}
	if spell.EquippedItemInvTypes != 0 {
		if isEnchant && invType == invTypeWeapon &&
			(spell.EquippedItemInvTypes&(1<<invTypeWeaponMainhand) != 0 || spell.EquippedItemInvTypes&(1<<invTypeWeaponOffhand) != 0) {
			return true
		}
		if (spell.EquippedItemInvTypes & (1 << invType)) == 0 {
			return false
		}
	}
	return true
}

// checkItemTargetCast mirrors the target-item arm of Spell::CheckItems
// (Spell.cpp:6748-6754). Item-target resolution mirrors
// SpellCastTargets::Update (Spell.cpp:462-478): TARGET_FLAG_ITEM resolves
// through Player::GetItemByGuid (Player.cpp:9994-10024 — the player's
// inventory, bank, and bags, mirrored by the character_inventory join);
// TARGET_FLAG_TRADE_ITEM carries the trade slot index rather than a GUID,
// and only TRADE_SLOT_NONTRADED (TradeSlots, TradeData.h:27) resolves —
// "also prevents hacking slots" — to the partner's non-traded trade item
// (TradeData::GetTraderData, Spell.cpp:471-474). An item GUID that resolves
// to nothing fails with SPELL_FAILED_ITEM_GONE; an item whose template
// does not fit the spell fails with SPELL_FAILED_EQUIPPED_ITEM_CLASS via
// Item::IsFitToSpellRequirements (Item.cpp:799-832). Neither failure
// carries extra WriteCastResultInfo params (Spell.cpp:3974-4160), so no
// castFailedExtParams case is needed. Client-initiated casts only —
// triggered casts go through castSpellDirect, not this path.
func (s *session) checkItemTargetCast(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	itemGUID := target.ItemGUID
	if itemGUID == 0 {
		return 0
	}
	var entry uint32
	resolved := false
	switch {
	case target.Flags&protocol.SpellTargetFlagItem != 0:
		if s != nil && s.server != nil && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
			var found int64
			if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
				`SELECT ii.itemEntry FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ci.item = ?`,
				s.playerGUID, int64(itemGUID)).Scan(&found); err == nil {
				entry, resolved = uint32(found), true
			}
		}
	case target.Flags&protocol.SpellTargetFlagTradeItem != 0:
		// The wire "GUID" is the trade slot index; only the non-traded
		// slot (6) resolves, anything else is ITEM_GONE.
		if itemGUID == tradeSlotNonTraded && s != nil && s.trade != nil &&
			s.trade.Partner != nil && s.trade.Partner.trade != nil {
			if slotItem, ok := s.trade.Partner.trade.Items[uint8(tradeSlotNonTraded)]; ok && slotItem.ItemEntry != 0 {
				entry, resolved = slotItem.ItemEntry, true
			}
		}
	}
	if !resolved {
		return spellFailedItemGone
	}
	// A resolved item whose template row is missing is a data gap, not a
	// client fault: unknown data is permissive (terrain.go convention).
	data, err := s.loadItemQueryData(ctx, entry)
	if err != nil {
		return 0
	}
	if !isItemFitToSpell(spell, data.Class, data.SubClass, data.InventoryType) {
		return spellFailedEquippedItemClass
	}
	return 0
}

// canNoReagentCast mirrors Player::CanNoReagentCast (Player.cpp:23971-23988):
// spells carrying SPELL_ATTR5_NO_REAGENT_WHILE_PREP cost no reagents while
// the caster has UNIT_FLAG_PREPARATION set (arena preparation). The second
// C++ arm (spellInfo->SpellFamilyFlags & the PLAYER_NO_REAGENT_COST_1..3
// update values) has no Go bridge: nothing applies SPELL_AURA_NO_REAGENT_USE
// (256) to a player, so those fields are never set and the mask arm is dead.
// Client-initiated casts only — triggered casts go through castSpellDirect,
// which runs no reagent check at all.
func (s *session) canNoReagentCast(spell wotlk.Spell) bool {
	if s == nil || s.player == nil {
		return false
	}
	return spell.AttributesEx5&spellAttr5NoReagentWhilePrep != 0 &&
		s.player.UnitFlags&unitFlagPreparation != 0
}

// tradeItemTargetCast returns true when the wire target resolves to a trade
// item that is not the caster's own — the Spell.cpp:6782-6785 arm of the
// reagent block: a non-own traded item (in the trader's trade slot) forces
// the reagent check even when Player::CanNoReagentCast would skip it. The
// resolution mirrors checkItemTargetCast: TARGET_FLAG_TRADE_ITEM carries the
// slot index and only TRADE_SLOT_NONTRADED (TradeData.h:27) resolves, to the
// partner's item — never the caster's.
func (s *session) tradeItemTargetCast(target protocol.SpellTargetData) bool {
	if target.Flags&protocol.SpellTargetFlagTradeItem == 0 || target.ItemGUID != tradeSlotNonTraded {
		return false
	}
	if s == nil || s.trade == nil || s.trade.Partner == nil || s.trade.Partner.trade == nil {
		return false
	}
	slotItem, ok := s.trade.Partner.trade.Items[uint8(tradeSlotNonTraded)]
	return ok && slotItem.ItemEntry != 0
}

// itemStoreTemplateInfo carries the item_template columns the CheckItems
// CREATE_ITEM arm needs: the max stack size (for the createCount clamp,
// Spell.cpp:6881) and the item-limit category (for the conjure carve-out,
// Spell.cpp:6887) — plus the columns the ENCHANT_ITEM / ENCHANT_ITEM_PRISMATIC
// arms need: item/required level (the exploit-fix level gate,
// Spell.cpp:6939), the socket colors (the prismatic-socket gate,
// Spell.cpp:6958-6965) and the item-spell id/trigger pairs (the usable-item
// scan, Spell.cpp:6943-6952).
type itemStoreTemplateInfo struct {
	Stackable               uint32
	LimitCategory           uint32
	ItemLevel               uint32
	RequiredLevel           uint32
	Class                   uint32
	Quality                 uint32
	RequiredDisenchantSkill uint32
	DisenchantID            uint32
	SocketColors            [3]uint32
	SpellIDs                [5]uint32
	SpellTriggers           [5]uint32
}

// getItemStoreTemplateInfo is a cached item_template lookup for the
// CheckItems CREATE_ITEM arm, following the getItemTemplateClassInfo
// pattern. Unknown entries are permissive only where C++ is (the template
// miss itself fails with SPELL_FAILED_ITEM_NOT_FOUND).
func (s *Server) getItemStoreTemplateInfo(ctx context.Context, entry uint32) (itemStoreTemplateInfo, bool) {
	if entry == 0 {
		return itemStoreTemplateInfo{}, false
	}
	s.itemStoreTemplateMu.RLock()
	if s.itemStoreTemplates != nil {
		if info, ok := s.itemStoreTemplates[entry]; ok {
			s.itemStoreTemplateMu.RUnlock()
			return info, true
		}
	}
	s.itemStoreTemplateMu.RUnlock()

	if s.WorldStore == nil || s.WorldStore.DB == nil {
		return itemStoreTemplateInfo{}, false
	}
	var stackable, limitCategory, itemLevel, requiredLevel uint32
	var class, quality, disenchantID uint32
	// RequiredDisenchantSkill defaults to -1 in the world item_template
	// table; scan signed so the negative value survives, then convert to
	// uint32 exactly like the C++ loader does (ItemTemplate.h), so the
	// uint32(-1) comparison in the DISENCHANT arm (Spell.cpp:7033) matches.
	var requiredDisenchantSkill int32
	var socketColors [3]uint32
	var spellIDs, spellTriggers [5]uint32
	err := s.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(stackable, 1), COALESCE(ItemLimitCategory, 0),
		COALESCE(ItemLevel, 0), COALESCE(RequiredLevel, 0),
		COALESCE(class, 0), COALESCE(Quality, 0),
		COALESCE(RequiredDisenchantSkill, -1), COALESCE(DisenchantID, 0),
		COALESCE(SocketColor_1, 0), COALESCE(SocketColor_2, 0), COALESCE(SocketColor_3, 0),
		COALESCE(spellid_1, 0), COALESCE(spelltrigger_1, 0),
		COALESCE(spellid_2, 0), COALESCE(spelltrigger_2, 0),
		COALESCE(spellid_3, 0), COALESCE(spelltrigger_3, 0),
		COALESCE(spellid_4, 0), COALESCE(spelltrigger_4, 0),
		COALESCE(spellid_5, 0), COALESCE(spelltrigger_5, 0)
		FROM item_template WHERE entry = ? LIMIT 1`, entry).Scan(
		&stackable, &limitCategory, &itemLevel, &requiredLevel,
		&class, &quality, &requiredDisenchantSkill, &disenchantID,
		&socketColors[0], &socketColors[1], &socketColors[2],
		&spellIDs[0], &spellTriggers[0],
		&spellIDs[1], &spellTriggers[1],
		&spellIDs[2], &spellTriggers[2],
		&spellIDs[3], &spellTriggers[3],
		&spellIDs[4], &spellTriggers[4])
	if err != nil {
		return itemStoreTemplateInfo{}, false
	}
	info := itemStoreTemplateInfo{Stackable: stackable, LimitCategory: limitCategory,
		ItemLevel: itemLevel, RequiredLevel: requiredLevel,
		Class: class, Quality: quality,
		RequiredDisenchantSkill: uint32(requiredDisenchantSkill), DisenchantID: disenchantID,
		SocketColors: socketColors, SpellIDs: spellIDs, SpellTriggers: spellTriggers}
	s.itemStoreTemplateMu.Lock()
	if s.itemStoreTemplates == nil {
		s.itemStoreTemplates = make(map[uint32]itemStoreTemplateInfo)
	}
	s.itemStoreTemplates[entry] = info
	s.itemStoreTemplateMu.Unlock()
	return info, true
}

// freeInventorySpace counts the player's free inventory slots (backpack +
// equipped bags), mirroring Player::GetFreeInventorySpace (Player.cpp) as
// used by the CREATE_ITEM_2 arm (Spell.cpp:6872). Slot ranges mirror
// freeInventorySlotForPlayer (items.go:1717).
func (s *session) freeInventorySpace(ctx context.Context, playerGUID uint64) uint32 {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	cdb := s.server.CharactersStore.DB
	freeIn := func(bagKey int64, first, last int64) uint32 {
		rows, err := cdb.QueryContext(ctx, "SELECT slot FROM character_inventory WHERE guid = ? AND bag = ?", playerGUID, bagKey)
		if err != nil {
			return 0
		}
		defer rows.Close()
		used := make(map[int64]struct{})
		for rows.Next() {
			var slot int64
			if rows.Scan(&slot) == nil {
				used[slot] = struct{}{}
			}
		}
		var free uint32
		for slot := first; slot <= last; slot++ {
			if _, ok := used[slot]; !ok {
				free++
			}
		}
		return free
	}
	n := freeIn(0, int64(invSlotItemStart), int64(invSlotItemEnd-1))
	for _, b := range s.getEquippedBags(ctx, playerGUID) {
		if s.server.WorldStore == nil || s.server.WorldStore.DB == nil {
			continue
		}
		var slots int64
		if err := s.server.WorldStore.DB.QueryRowContext(ctx, `SELECT COALESCE(ContainerSlots, 0) FROM item_template WHERE entry = (SELECT itemEntry FROM item_instance WHERE guid = ?)`, b.guid).Scan(&slots); err != nil || slots <= 0 {
			continue
		}
		last := int64(35)
		if slots-1 < last {
			last = slots - 1
		}
		n += freeIn(b.guid, 0, last)
	}
	return n
}

// ownedItemCount mirrors the count half of Player::HasItemCount
// (Player.cpp:1079) over the character's inventory (bank excluded, the
// conjure arm's default).
func (s *session) ownedItemCount(ctx context.Context, playerGUID uint64, entry uint32) uint32 {
	if s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return 0
	}
	var total int64
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(ii.count), 0) FROM character_inventory AS ci
		JOIN item_instance AS ii ON ii.guid = ci.item
		WHERE ci.guid = ? AND ii.itemEntry = ?`, playerGUID, entry).Scan(&total); err != nil || total < 0 {
		return 0
	}
	return uint32(total)
}

// canStoreNewItem is a non-mutating probe of the inventory-space terms of
// Player::CanStoreNewItem (Player.cpp) for the CheckItems CREATE_ITEM call
// (NULL_BAG/NULL_SLOT): free space in existing partial stacks absorbs what
// it can, the remainder needs free slots. Bag-family, soulbound and other
// placement rules have no Go model; the probe answers the space question
// the arm actually gates on.
func (s *session) canStoreNewItem(ctx context.Context, playerGUID uint64, entry uint32, count uint32) uint8 {
	info, ok := s.server.getItemStoreTemplateInfo(ctx, entry)
	if !ok {
		return equipErrItemNotFound
	}
	maxStack := info.Stackable
	if maxStack < 1 {
		maxStack = 1
	}
	remaining := count
	if maxStack > 1 && s.server.CharactersStore != nil && s.server.CharactersStore.DB != nil {
		var space int64
		_ = s.server.CharactersStore.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(? - ii.count), 0) FROM character_inventory AS ci
			JOIN item_instance AS ii ON ii.guid = ci.item
			WHERE ci.guid = ? AND ii.itemEntry = ? AND ii.count < ?`, maxStack, playerGUID, entry, maxStack).Scan(&space)
		if space > 0 {
			if uint64(remaining) <= uint64(space) {
				return equipErrOk
			}
			remaining -= uint32(space)
		}
	}
	if need := (remaining + maxStack - 1) / maxStack; s.freeInventorySpace(ctx, playerGUID) < need {
		return equipErrInvFull
	}
	return equipErrOk
}

// checkSpellCreateItemCast mirrors the SPELL_EFFECT_CREATE_ITEM /
// SPELL_EFFECT_CREATE_ITEM_2 arm of the Spell::CheckItems special-effects
// loop (Spell.cpp:6864-6905). Returns the SpellCastResult failure code, or
// 0 when the arm passes. Client-initiated casts only — triggered casts go
// through castSpellDirect (the !IsTriggered() arm is structural).
func (s *session) checkSpellCreateItemCast(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	for i := 0; i < len(spell.Effects); i++ {
		eff := spell.Effects[i]
		if eff.Effect != spellEffectCreateItem && eff.Effect != spellEffectCreateItem2 {
			continue
		}
		if eff.ItemType == 0 {
			continue
		}
		// m_targets.GetUnitTarget() means explicit cast, otherwise the
		// caster (Spell.cpp:6867); the gate applies only to player targets
		// (TYPEID_PLAYER) — creature targets carry no inventory model.
		ts := s
		if target.UnitGUID != 0 && target.UnitGUID != s.playerGUID {
			uts := s.server.findSessionByGUID(target.UnitGUID)
			if uts == nil || uts.player == nil {
				continue
			}
			ts = uts
		}
		// SPELL_EFFECT_CREATE_ITEM_2 picks its item from a pool, so the
		// cast needs at least one free inventory slot up front
		// (Spell.cpp:6871-6876).
		if eff.Effect == spellEffectCreateItem2 && ts.freeInventorySpace(ctx, ts.playerGUID) == 0 {
			s.sendEquipError(equipErrInvFull, 0) // player->SendEquipError (the caster)
			return spellFailedDontReport
		}
		info, ok := s.server.getItemStoreTemplateInfo(ctx, eff.ItemType)
		if !ok {
			return spellFailedItemNotFound
		}
		// std::clamp(CalcValue(), 1, maxStackSize) (Spell.cpp:6881).
		createCount := s.spellEffectCheckCastValue(spell, eff, i)
		if createCount < 1 {
			createCount = 1
		}
		if maxStack := int32(info.Stackable); maxStack > 0 && createCount > maxStack {
			createCount = maxStack
		}
		if msg := ts.canStoreNewItem(ctx, ts.playerGUID, eff.ItemType, uint32(createCount)); msg != equipErrOk {
			if info.LimitCategory == 0 {
				s.sendEquipError(msg, 0)
				return spellFailedDontReport
			}
			// Conjure Food/Water/Refreshment (Spell.cpp:6890-6902): mage
			// conjure spells whose created item the target already owns
			// summon the refreshment table (Effects[EFFECT_1]) instead of
			// failing with TOO_MANY_OF_ITEM.
			if spell.SpellFamilyName != spellFamilyMage || spell.SpellFamilyFlags[0]&spellFamilyFlagConjureRefreshment == 0 {
				return spellFailedTooManyOfItem
			}
			if ts.ownedItemCount(ctx, ts.playerGUID, eff.ItemType) == 0 {
				s.sendEquipError(msg, 0)
				return spellFailedDontReport
			}
			if tableSpell := s.spellEffectCheckCastValue(spell, spell.Effects[1], 1); tableSpell > 0 {
				s.castSpellDirect(ctx, uint32(tableSpell), s.playerGUID)
			}
			return spellFailedDontReport
		}
	}
	return 0
}

// enchantItemTarget carries the resolution of the wire item target for the
// ENCHANT_ITEM / ENCHANT_ITEM_PRISMATIC CheckItems arms: the template entry,
// the item_instance guid (for reading enchantments), and whether the caster
// owns the item (Item::GetOwner() == player — trade-window targets resolve
// to the partner's non-traded slot item, which the caster does not own).
type enchantItemTarget struct {
	entry         uint32
	instanceGUID  uint64
	ownedByCaster bool
}

// resolveEnchantItemTarget mirrors SpellCastTargets::Update (Spell.cpp:462-478)
// for the enchant arms: TARGET_FLAG_ITEM resolves through the player's own
// inventory (the Player::GetItemByGuid arm, Player.cpp:9994-10024);
// TARGET_FLAG_TRADE_ITEM carries the trade slot index rather than a GUID and
// only TRADE_SLOT_NONTRADED (TradeData.h:27) resolves, to the partner's item.
func (s *session) resolveEnchantItemTarget(ctx context.Context, target protocol.SpellTargetData) (enchantItemTarget, bool) {
	if target.ItemGUID == 0 {
		return enchantItemTarget{}, false
	}
	if s == nil || s.server == nil || s.server.CharactersStore == nil || s.server.CharactersStore.DB == nil {
		return enchantItemTarget{}, false
	}
	cdb := s.server.CharactersStore.DB
	switch {
	case target.Flags&protocol.SpellTargetFlagItem != 0:
		var found int64
		if err := cdb.QueryRowContext(ctx,
			`SELECT ii.itemEntry FROM character_inventory ci JOIN item_instance ii ON ii.guid = ci.item WHERE ci.guid = ? AND ci.item = ?`,
			s.playerGUID, int64(target.ItemGUID)).Scan(&found); err != nil {
			return enchantItemTarget{}, false
		}
		return enchantItemTarget{entry: uint32(found), instanceGUID: target.ItemGUID, ownedByCaster: true}, true
	case target.Flags&protocol.SpellTargetFlagTradeItem != 0:
		if target.ItemGUID != tradeSlotNonTraded || s.trade == nil ||
			s.trade.Partner == nil || s.trade.Partner.trade == nil {
			return enchantItemTarget{}, false
		}
		slotItem, ok := s.trade.Partner.trade.Items[uint8(tradeSlotNonTraded)]
		if !ok || slotItem.ItemEntry == 0 {
			return enchantItemTarget{}, false
		}
		return enchantItemTarget{entry: slotItem.ItemEntry, instanceGUID: slotItem.ItemGUID, ownedByCaster: false}, true
	}
	return enchantItemTarget{}, false
}

// enchantTargetPrismaticID reads the item_instance enchantments column and
// returns the PRISMATIC_ENCHANTMENT_SLOT (slot 6, ItemDefines.h:152)
// enchantment id — the GetEnchantmentId(PRISMATIC_ENCHANTMENT_SLOT) test
// (Spell.cpp:6964). The column holds 36 space-separated ints, 3 per slot
// (id, duration, charges), so slot 6's id is index 18. An unreadable column
// behaves as no prismatic enchantment (terrain.go convention).
func (s *session) enchantTargetPrismaticID(ctx context.Context, instanceGUID uint64) uint32 {
	if s == nil || s.server == nil || s.server.CharactersStore == nil ||
		s.server.CharactersStore.DB == nil || instanceGUID == 0 {
		return 0
	}
	var raw string
	if err := s.server.CharactersStore.DB.QueryRowContext(ctx,
		`SELECT COALESCE(enchantments, '') FROM item_instance WHERE guid = ? LIMIT 1`,
		int64(instanceGUID)).Scan(&raw); err != nil {
		return 0
	}
	fields := strings.Fields(raw)
	if len(fields) <= prismaticEnchantIdx {
		return 0
	}
	if id, err := strconv.ParseUint(fields[prismaticEnchantIdx], 10, 32); err == nil {
		return uint32(id)
	}
	return 0
}

// checkSpellEnchantItemCast mirrors the SPELL_EFFECT_ENCHANT_ITEM /
// SPELL_EFFECT_ENCHANT_ITEM_PRISMATIC arm of the Spell::CheckItems
// special-effects loop (Spell.cpp:6917-6994). Returns the SpellCastResult
// failure code, or 0 when the arm passes. Client-initiated casts only —
// triggered casts go through castSpellDirect.
func (s *session) checkSpellEnchantItemCast(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	for i := 0; i < len(spell.Effects); i++ {
		eff := spell.Effects[i]
		isEnchant := eff.Effect == spellEffectEnchantItem
		isPrismatic := eff.Effect == spellEffectEnchantItemPrismatic
		if !isEnchant && !isPrismatic {
			continue
		}
		t, resolved := s.resolveEnchantItemTarget(ctx, target)
		// The ENCHANT_ITEM vellum sub-arm (Spell.cpp:6918-6933): the scroll
		// (Effects[i].ItemType) is only created when a vellum is the target.
		if isEnchant && eff.ItemType != 0 && resolved {
			if classInfo, ok := s.getItemTemplateClassInfo(ctx, t.entry); ok &&
				classInfo.Class == itemClassTradeGoods &&
				(classInfo.SubClass == itemSubclassArmorEnchantment ||
					classInfo.SubClass == itemSubclassWeaponEnchantment) {
				// cannot enchant vellum for other player (Spell.cpp:6921-6922)
				if !t.ownedByCaster {
					return spellFailedNotTradeable
				}
				// The m_CastItem NO_REAGENT_COST exploit guard
				// (Spell.cpp:6924-6925) is structural: handleCastSpell never
				// carries a cast item (item casts run through handleUseItem).
				// Room for the created scroll (Spell.cpp:6926-6932).
				if msg := s.canStoreNewItem(ctx, s.playerGUID, eff.ItemType, 1); msg != equipErrOk {
					s.sendEquipError(msg, 0)
					return spellFailedDontReport
				}
			}
		}
		// The shared ENCHANT_ITEM (fallthrough) / ENCHANT_ITEM_PRISMATIC arm
		// (Spell.cpp:6934-6994).
		if !resolved {
			return spellFailedItemNotFound
		}
		// A missing template row is a data gap, not a client fault:
		// unknown data is permissive (terrain.go convention).
		info, ok := s.server.getItemStoreTemplateInfo(ctx, t.entry)
		if !ok {
			continue
		}
		// required level has to be checked also! Exploit fix (Spell.cpp:6939).
		if info.ItemLevel < spell.BaseLevel ||
			(info.RequiredLevel != 0 && info.RequiredLevel < spell.BaseLevel) {
			return spellFailedLowLevel
		}
		// isItemUsable: any item spell with an on-use trigger
		// (Spell.cpp:6941-6952).
		isItemUsable := false
		for k := 0; k < maxItemProtoSpells; k++ {
			if info.SpellIDs[k] > 0 && (info.SpellTriggers[k] == itemSpellTriggerOnUse ||
				info.SpellTriggers[k] == itemSpellTriggerOnNoDelayUse) {
				isItemUsable = true
				break
			}
		}
		// sSpellItemEnchantmentStore.LookupEntry(Effects[i].MiscValue)
		// (Spell.cpp:6954); a missing entry only fails inside the
		// trade-slot arm below, matching C++.
		var enchantEntry wotlk.SpellItemEnchantmentEntry
		hasEnchantEntry := false
		if eff.MiscValue > 0 && s.server.Data != nil {
			if e, found, err := s.server.Data.SpellItemEnchantment(uint32(eff.MiscValue)); err == nil && found {
				enchantEntry, hasEnchantEntry = e, true
			}
		}
		if hasEnchantEntry {
			for k := 0; k < maxItemEnchantEffects && k < len(enchantEntry.Effects); k++ {
				switch enchantEntry.Effects[k] {
				case itemEnchantTypeUseSpell:
					// do not allow adding usable enchantments to items that
					// have use effect already (Spell.cpp:6959-6962).
					if isItemUsable {
						return spellFailedOnUseEnchant
					}
				case itemEnchantTypePrismaticSock:
					// ITEM_ENCHANTMENT_TYPE_PRISMATIC_SOCKET
					// (Spell.cpp:6963-6968): the item already has all three
					// sockets or already carries a prismatic enchantment.
					numSockets := uint32(0)
					for c := 0; c < maxItemProtoSockets && c < len(info.SocketColors); c++ {
						if info.SocketColors[c] != 0 {
							numSockets++
						}
					}
					if numSockets == maxItemProtoSockets ||
						s.enchantTargetPrismaticID(ctx, t.instanceGUID) != 0 {
						return spellFailedMaxSockets
					}
				}
			}
		}
		// Not allow enchant in trade slot for some enchant type
		// (Spell.cpp:6970-6981).
		if !t.ownedByCaster {
			if !hasEnchantEntry {
				return spellFailedError
			}
			if enchantEntry.Flags&enchantFlagCanSoulbound != 0 {
				return spellFailedNotTradeable
			}
		}
	}
	return 0
}

// checkSpellEnchantItemTemporaryCast mirrors the
// SPELL_EFFECT_ENCHANT_ITEM_TEMPORARY arm of the Spell::CheckItems
// special-effects loop (Spell.cpp:6996-7011). Returns the SpellCastResult
// failure code, or 0 when the arm passes. Client-initiated casts only —
// triggered casts go through castSpellDirect.
func (s *session) checkSpellEnchantItemTemporaryCast(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	for i := 0; i < len(spell.Effects); i++ {
		eff := spell.Effects[i]
		if eff.Effect != spellEffectEnchantItemTemporary {
			continue
		}
		t, resolved := s.resolveEnchantItemTarget(ctx, target)
		if !resolved {
			return spellFailedItemNotFound
		}
		// Not allow enchant in trade slot for some enchant type
		// (Spell.cpp:7000-7008).
		if !t.ownedByCaster {
			// sSpellItemEnchantmentStore.LookupEntry(Effects[i].MiscValue);
			// a missing entry only fails inside the trade-slot arm, matching C++.
			var enchantEntry wotlk.SpellItemEnchantmentEntry
			hasEnchantEntry := false
			if eff.MiscValue > 0 && s.server.Data != nil {
				if e, found, err := s.server.Data.SpellItemEnchantment(uint32(eff.MiscValue)); err == nil && found {
					enchantEntry, hasEnchantEntry = e, true
				}
			}
			if !hasEnchantEntry {
				return spellFailedError
			}
			if enchantEntry.Flags&enchantFlagCanSoulbound != 0 {
				return spellFailedNotTradeable
			}
		}
		// The item-level restriction arm (Spell.cpp:7010-7011, the m_CastItem ×
		// MaxLevel LOWLEVEL/HIGHLEVEL gates) is structural: handleCastSpell
		// never carries a cast item (item casts run through handleUseItem,
		// which runs no CheckCast gates).
	}
	return 0
}

// checkSpellDisenchantCast mirrors the SPELL_EFFECT_DISENCHANT arm of the
// Spell::CheckItems special-effects loop (Spell.cpp:7025-7052). Returns the
// SpellCastResult failure code, or 0 when the arm passes.
// Client-initiated casts only — triggered casts go through castSpellDirect.
func (s *session) checkSpellDisenchantCast(ctx context.Context, spell wotlk.Spell, target protocol.SpellTargetData) uint8 {
	for i := 0; i < len(spell.Effects); i++ {
		if spell.Effects[i].Effect != spellEffectDisenchant {
			continue
		}
		t, resolved := s.resolveEnchantItemTarget(ctx, target)
		if !resolved {
			return spellFailedCantBeDisenchanted
		}
		// Prevent disenchanting in trade slot: a trade-window target resolves
		// to the partner's non-traded slot item, whose owner is not the
		// caster (Spell.cpp:7027-7030).
		if !t.ownedByCaster {
			return spellFailedCantBeDisenchanted
		}
		// Missing item_template row (Spell.cpp:7032-7034).
		info, ok := s.server.getItemStoreTemplateInfo(ctx, t.entry)
		if !ok {
			return spellFailedCantBeDisenchanted
		}
		// 2.0.x addon: the item cannot be disenchanted at all
		// (Spell.cpp:7036-7038, RequiredDisenchantSkill == uint32(-1)).
		if info.RequiredDisenchantSkill == 0xFFFFFFFF {
			return spellFailedCantBeDisenchanted
		}
		// 2.0.x addon: player enchanting level against the item's
		// disenchanting requirement (Spell.cpp:7039-7040).
		if skill := playerSkillTotalValue(s.player, skillEnchanting); skill < 0 || info.RequiredDisenchantSkill > uint32(skill) {
			return spellFailedLowCastlevel
		}
		// Quality 2-4 only (Spell.cpp:7041-7042).
		if info.Quality > 4 || info.Quality < 2 {
			return spellFailedCantBeDisenchanted
		}
		// Weapon or armor class only (Spell.cpp:7043-7044).
		if info.Class != itemClassWeapon && info.Class != itemClassArmor {
			return spellFailedCantBeDisenchanted
		}
		// Needs a disenchant loot entry (Spell.cpp:7045-7046).
		if info.DisenchantID == 0 {
			return spellFailedCantBeDisenchanted
		}
	}
	return 0
}

// checkSpellEquippedItemRequirements validates equipped weapon and armor requirements for spells before cast execution.
// References: TrinityCore Spell::CheckCast (Spell.cpp:6768-6775, 7206-7237) and Player::HasItemFitToSpellRequirements (Player.cpp:23916-23969).
func (s *session) checkSpellEquippedItemRequirements(ctx context.Context, spell wotlk.Spell) (uint8, bool) {
	if spell.EquippedItemClass < 0 || s == nil || s.player == nil || s.player.Equipment == "" {
		return 0, true
	}

	// 1. Main hand weapon requirement (SPELL_ATTR3_MAIN_HAND)
	if spell.AttributesEx3&spellAttr3MainHand != 0 {
		mainHandEntry := s.getEquipmentItem(equipSlotMainhand)
		if mainHandEntry == 0 {
			return spellFailedEquippedItemClass, false
		}
		if info, ok := s.getItemTemplateClassInfo(ctx, mainHandEntry); ok {
			if !isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
				return spellFailedEquippedItemClass, false
			}
		}
	}

	// 2. Offhand weapon requirement (SPELL_ATTR3_REQ_OFFHAND)
	if spell.AttributesEx3&spellAttr3ReqOffhand != 0 {
		offHandEntry := s.getEquipmentItem(equipSlotOffhand)
		if offHandEntry == 0 {
			return spellFailedEquippedItemClass, false
		}
		if info, ok := s.getItemTemplateClassInfo(ctx, offHandEntry); ok {
			if !isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
				return spellFailedEquippedItemClass, false
			}
		}
	}

	// 3. General item class requirements (ITEM_CLASS_WEAPON, ITEM_CLASS_ARMOR)
	switch spell.EquippedItemClass {
	case itemClassWeapon:
		if spell.AttributesEx3&(spellAttr3MainHand|spellAttr3ReqOffhand) == 0 {
			weaponSlots := []uint8{equipSlotMainhand, equipSlotOffhand, equipSlotRanged}
			hasFitWeapon := false
			for _, slot := range weaponSlots {
				entry := s.getEquipmentItem(slot)
				if entry == 0 {
					continue
				}
				info, ok := s.getItemTemplateClassInfo(ctx, entry)
				if !ok || isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
					hasFitWeapon = true
					break
				}
			}
			if !hasFitWeapon {
				return spellFailedEquippedItemClass, false
			}
		}

	case itemClassArmor:
		// Shield requirement (subclass 6 = shield, subclass 5 = buckler)
		if spell.EquippedItemSubClass&((1<<itemSubclassArmorBuckler)|(1<<itemSubclassArmorShield)) != 0 {
			offhandEntry := s.getEquipmentItem(equipSlotOffhand)
			if offhandEntry == 0 {
				return spellFailedEquippedItemClass, false
			}
			if info, ok := s.getItemTemplateClassInfo(ctx, offhandEntry); ok {
				if !isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
					return spellFailedEquippedItemClass, false
				}
			}
		} else {
			hasFitArmor := false
			for slot := uint8(0); slot < equipSlotEnd; slot++ {
				if slot == equipSlotMainhand || slot == equipSlotTabard {
					continue
				}
				entry := s.getEquipmentItem(slot)
				if entry == 0 {
					continue
				}
				info, ok := s.getItemTemplateClassInfo(ctx, entry)
				if !ok || isItemFitToSpell(spell, info.Class, info.SubClass, info.InvType) {
					hasFitArmor = true
					break
				}
			}
			if !hasFitArmor {
				return spellFailedEquippedItemClass, false
			}
		}
	}

	return 0, true
}

func (s *session) handleEffectCreateItem(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if eff.ItemType == 0 {
		return
	}
	playerGUID := targetGUID
	if playerGUID == 0 {
		playerGUID = s.playerGUID
	}
	if playerGUID == 0 {
		return
	}
	count := int32(eff.BasePoints + 1)
	if count < 1 {
		count = 1
	}
	if _, err := s.storeOrStackItem(ctx, playerGUID, eff.ItemType, uint32(count)); err != nil {
		return
	}
}

func (s *session) handleEffectResurrect(ctx context.Context, targetGUID uint64, spell wotlk.Spell, eff wotlk.SpellEffect) {
	if targetGUID == 0 || s.server == nil {
		return
	}
	targetSess := s.server.findSessionByGUID(targetGUID)
	if targetSess == nil || targetSess.player == nil {
		return
	}
	if targetSess.player.Health > 0 {
		return
	}
	if targetSess.resurrection != nil {
		return
	}
	healthPct := eff.BasePoints + 1
	if healthPct < 1 {
		healthPct = 1
	}
	if healthPct > 100 {
		healthPct = 100
	}
	maxHealth := targetSess.player.MaxHealth
	if maxHealth == 0 {
		maxHealth = 1
	}
	health := uint32(int64(maxHealth) * int64(healthPct) / 100)
	maxMana := targetSess.player.MaxPowers[0]
	mana := uint32(int64(maxMana) * int64(healthPct) / 100)
	targetSess.setResurrectRequestData(s.playerGUID, 0, 0, 0, 0, health, mana)
	targetSess.sendResurrectRequest(s.playerGUID, "", false, false)
}

func (s *session) handleEffectHealthLeech(ctx context.Context, spellID uint32, hitTargets []uint64, effIndex int, eff wotlk.SpellEffect) {
	if s.playerGUID == 0 {
		return
	}
	amount := eff.BasePoints + 1
	if amount < 0 {
		return
	}
	damageAmount := uint32(amount)
	for _, target := range hitTargets {
		if target == 0 || target == s.playerGUID {
			continue
		}
		// SpellEffects.cpp:1548 (-GetHealthGain(-damage)): the leech heal is the
		// damage the target actually lost — post-absorb, overkill excluded —
		// scaled by the effect value multiplier, not the rolled damage.
		healthBefore := uint32(0)
		if tgt, ok := s.getCombatTarget(ctx, target); ok {
			healthBefore = tgt.Health
		}
		dealt := s.executeSpellDamage(ctx, target, spellID, damageAmount, effIndex)
		if dealt > healthBefore {
			dealt = healthBefore
		}
		s.executeSpellHeal(ctx, s.playerGUID, spellID, effectValueMultiplied(dealt, eff.Amplitude), effIndex)
	}
}
