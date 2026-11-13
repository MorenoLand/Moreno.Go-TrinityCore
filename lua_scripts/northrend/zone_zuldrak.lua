-- Zul'Drak zone spell aura scripts -- Lua ports of the AuraScript arms
-- in src/server/scripts/Northrend/zone_zuldrak.cpp.
--
-- PORTED (tick 197, RegisterAuraEvent OnApply = 2 / OnRemove = 3):
--   spell_scourge_disguise (51966, zone_zuldrak.cpp:966-981):
--   EFFECT_0 TRANSFORM. OnApply the target casts Scourge Disguise
--   Instability (51971) on itself, triggered; OnRemove the target
--   removes that instability aura (C++ plain RemoveAura). No Load
--   gates in C++; no PreventDefaultAction. This supersedes the
--   "no AuraScript bridge — documented-only" note in
--   npc_storm_cloud.lua, which predates the tick-196
--   RegisterAuraEvent hook.
-- PORTED (tick 424, RegisterAuraEvent OnPeriodic = 1):
--   spell_random_ingredient_aura (51015 / 51154 / 51157,
--   zone_zuldrak.cpp:657-671): EFFECT_0 SPELL_AURA_PERIODIC_
--   DUMMY. Each tick the target self-casts the matching Random
--   Ingredient spell (51134 / 51105 / 51107), non-triggered.
--   No PreventDefaultAction in C++ (PERIODIC_DUMMY has no
--   default tick action to suppress).
-- PORTED (tick 426, RegisterAuraEvent OnPeriodic = 1):
--   spell_scourge_disguise_instability (51971,
--   zone_zuldrak.cpp:984-1012): EFFECT_0 SPELL_AURA_DUMMY. Each
--   tick the target self-casts Scourge Disguise Expiring (52010),
--   triggered. DOCUMENTED DEVIATION: C++ CalcPeriodic re-rolls the
--   amplitude to irand(30, 240)s after every tick
--   (HandleUpdatePeriodic -> CalculatePeriodic) and no
--   CalcPeriodic hook exists in Go, so the tick interval here is
--   whatever the aura engine delivers from the DBC amplitude — the
--   cast leg is exact, the timing is not.
-- PORTED (RegisterSpellEvent OnEffectHitTarget = 5):
--   spell_random_ingredient (51134 / 51105 / 51107,
--   zone_zuldrak.cpp:689-766): EFFECT_0 SPELL_EFFECT_SCRIPT_
--   EFFECT. Hit player rolls urand by spell id, Finklestein
--   (28205, 25yd) casts FetchIngredients[i][0] on the player,
--   triggered, and Talk(FetchIngredients[i][3], player).
--   Ev5 player-cast fire site is live (finishSpellCast fires
--   fireSpellEffectHitTarget, spells.go:14892) — the old
--   "finishSpellCast never fires ev5" blocker in
--   npc_storm_cloud.lua is stale. NOTE: creature:CastSpell
--   threads the triggered flag via castCreatureSpellTriggered
--   (lua_creature_events.go:968 -> boss_ai.go:511), so the
--   older "flag accepted but not threaded" note is stale for
--   the creature path.
-- PORTED (tick 425, RegisterAuraEvent OnRemove = 3):
--   spell_fetch_ingredient_aura (zone_zuldrak.cpp:866-899): one
--   shared class covers all 21 fetch auras (51018/51055/51057/51059/
--   51062/51064/51067/51069/51072/51077/51079 easy, 51081/51083/51085/
--   51087/51091 medium, 51093/51095/51097/51100/51102 hard).
--   Expire-only + invisbuff 51216 + Finklestein 28205 within 100yd ->
--   drop the invisbuff, Finklestein Talk(SAY_RUINED 4). DORMANT: the
--   sender spell_random_ingredient (:676-755) is an unported
--   SpellScript, so no fetch aura lands yet.
--   spell_scourge_disguise_expiring (52010, :1014-1030): OnApply
--   whispers TEXT_DISGUISE_WARNING (28891) to the player target —
--   PORTED (tick 204, below) via the new player:WhisperText bridge
--   (Unit::Whisper textId overload, lua_player.go).
-- KNOWN LIMIT: player:CastSpell ignores the triggered flag
-- (lua_player.go:2577 -> castSpellDirect), so the disguised
-- player's triggered self-cast lands non-triggered; pre-existing
-- bridge gap, same as all prior player-target ports.

local SPELL_SCOURGE_DISGUISE = 51966 -- zone_zuldrak.cpp:957
local SPELL_SCOURGE_DISGUISE_INSTABILITY = 51971 -- zone_zuldrak.cpp:958
local SPELL_SCOURGE_DISGUISE_EXPIRING = 52010 -- zone_zuldrak.cpp:959
local TEXT_DISGUISE_WARNING = 28891 -- zone_zuldrak.cpp:961
local SPELL_RANDOM_INGREDIENT_EASY_AURA = 51015 -- zone_zuldrak.cpp:368
local SPELL_RANDOM_INGREDIENT_MEDIUM_AURA = 51154 -- zone_zuldrak.cpp:369
local SPELL_RANDOM_INGREDIENT_HARD_AURA = 51157 -- zone_zuldrak.cpp:370
local SPELL_RANDOM_INGREDIENT_EASY = 51134 -- zone_zuldrak.cpp:371
local SPELL_RANDOM_INGREDIENT_MEDIUM = 51105 -- zone_zuldrak.cpp:372
local SPELL_RANDOM_INGREDIENT_HARD = 51107 -- zone_zuldrak.cpp:373

-- C++ spell_scourge_disguise::ApplyEffect (zone_zuldrak.cpp:969-972).
local function scourgeDisguiseApply(event, aura, target)
    target:CastSpell(target, SPELL_SCOURGE_DISGUISE_INSTABILITY, true)
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE, 2, scourgeDisguiseApply)

-- C++ spell_scourge_disguise::RemoveEffect (zone_zuldrak.cpp:974-977).
local function scourgeDisguiseRemove(event, aura, target)
    target:RemoveAura(SPELL_SCOURGE_DISGUISE_INSTABILITY)
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE, 3, scourgeDisguiseRemove)

-- C++ spell_scourge_disguise_instability::HandleDummyTick
-- (zone_zuldrak.cpp:995-998): OnEffectPeriodic, EFFECT_0
-- SPELL_AURA_DUMMY: the target self-casts Scourge Disguise Expiring
-- (52010), triggered. No PreventDefaultAction in C++ (DUMMY has no
-- default tick action to suppress). DOCUMENTED DEVIATION: the
-- C++ tick interval is re-rolled to irand(30, 240)s after every
-- tick (CalcPeriodic :989-993 + HandleUpdatePeriodic :1000-1003),
-- and no CalcPeriodic hook exists in Go — the interval here is
-- whatever the aura engine delivers from the DBC amplitude.
local function scourgeDisguiseInstabilityTick(event, aura, target)
    target:CastSpell(target, SPELL_SCOURGE_DISGUISE_EXPIRING, true)
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE_INSTABILITY, 1, scourgeDisguiseInstabilityTick)

-- C++ spell_scourge_disguise_expiring::OnApply (zone_zuldrak.cpp:1018-1025):
-- if the player target still has the disguise, whisper the warning text
-- to them (Unit::Whisper textId overload, bossWhisper=true -> 0x2A).
-- DELTA: C++ localizes broadcast_text by target locale + sender gender;
-- the Go bridge reads the enUS Text column only.
local function scourgeDisguiseExpiringApply(event, aura, target)
    if target:GetObjectType() ~= "Player" then
        return
    end
    if target:HasAura(SPELL_SCOURGE_DISGUISE) then
        target:WhisperText(TEXT_DISGUISE_WARNING, target, true)
    end
end

RegisterAuraEvent(SPELL_SCOURGE_DISGUISE_EXPIRING, 2, scourgeDisguiseExpiringApply)

-- C++ spell_random_ingredient_aura::PeriodicTick
-- (zone_zuldrak.cpp:657-671): EFFECT_0 SPELL_AURA_PERIODIC_DUMMY.
-- Each tick the target self-casts the matching Random Ingredient
-- spell, non-triggered, keyed on the aura's own spell id
-- (C++ switch on GetSpellInfo()->Id; aura:GetAuraId() is the twin).
local function randomIngredientTick(event, aura, target)
    local cast = nil
    local spellId = aura:GetAuraId()
    if spellId == SPELL_RANDOM_INGREDIENT_EASY_AURA then
        cast = SPELL_RANDOM_INGREDIENT_EASY
    elseif spellId == SPELL_RANDOM_INGREDIENT_MEDIUM_AURA then
        cast = SPELL_RANDOM_INGREDIENT_MEDIUM
    elseif spellId == SPELL_RANDOM_INGREDIENT_HARD_AURA then
        cast = SPELL_RANDOM_INGREDIENT_HARD
    end
    if cast ~= nil and target ~= nil then
        target:CastSpell(target, cast)
    end
end

RegisterAuraEvent(SPELL_RANDOM_INGREDIENT_EASY_AURA, 1, randomIngredientTick)
RegisterAuraEvent(SPELL_RANDOM_INGREDIENT_MEDIUM_AURA, 1, randomIngredientTick)
RegisterAuraEvent(SPELL_RANDOM_INGREDIENT_HARD_AURA, 1, randomIngredientTick)

-- C++ spell_fetch_ingredient_aura_AuraScript::OnRemove
-- (zone_zuldrak.cpp:878-889): AfterEffectRemove, EFFECT_0
-- SPELL_AURA_DUMMY, AURA_EFFECT_HANDLE_REAL. Expire-only, and only
-- while the target still carries the apprentice invisbuff (51216)
-- and Finklestein (28205) is within 100yd: drop the invisbuff and
-- have Finklestein Talk(SAY_RUINED) to the target. C++ arm order
-- (expire gate, invisbuff gate, creature-found gate) preserved.
-- PORTED (below). DORMANT-BY-CONSTRUCTION: no Spell.dbc
-- records for 51134/51105/51107 yet, so the sender never fires
-- until the DBC ships; this handler is exact and goes live then.
-- The expire-only gate also depends on the aura duration, which
-- comes from Spell.dbc (absent here).
local AURA_REMOVE_BY_EXPIRE = 4 -- SpellAuraDefines.h:67; teron_gorefiend.lua:121 precedent
local NPC_FINKLESTEIN = 28205 -- zone_zuldrak.cpp:340
local SPELL_ALCHEMIST_APPRENTICE_INVISBUFF = 51216 -- zone_zuldrak.cpp:365
local SAY_RUINED = 4 -- zone_zuldrak.cpp:447

local function fetchIngredientRemove(event, aura, target)
    if aura:GetRemoveMode() ~= AURA_REMOVE_BY_EXPIRE then
        return
    end
    if not target:HasAura(SPELL_ALCHEMIST_APPRENTICE_INVISBUFF) then
        return
    end
    local finklestein = target:FindNearestCreature(NPC_FINKLESTEIN, 100.0)
    if finklestein == nil then
        return
    end
    target:RemoveAura(SPELL_ALCHEMIST_APPRENTICE_INVISBUFF)
    finklestein:Talk(SAY_RUINED, target)
end

-- The 21 FETCH spell ids (zone_zuldrak.cpp:378-402, in
-- FetchIngredients table order :610-632): 11 easy, 5 medium, 5 hard.
-- Easy
RegisterAuraEvent(51018, 3, fetchIngredientRemove) -- SPELL_FETCH_KNOTROOT
RegisterAuraEvent(51055, 3, fetchIngredientRemove) -- SPELL_FETCH_PICKLED_EAGLE_EGG
RegisterAuraEvent(51057, 3, fetchIngredientRemove) -- SPELL_FETCH_SPECKLED_GUANO
RegisterAuraEvent(51059, 3, fetchIngredientRemove) -- SPELL_FETCH_WITHERED_BATWING
RegisterAuraEvent(51062, 3, fetchIngredientRemove) -- SPELL_FETCH_SEASONED_SLIDER_CIDER
RegisterAuraEvent(51064, 3, fetchIngredientRemove) -- SPELL_FETCH_PULVERIZED_GARGOYLE_TEETH
RegisterAuraEvent(51067, 3, fetchIngredientRemove) -- SPELL_FETCH_MUDDY_MIRE_MAGGOT
RegisterAuraEvent(51069, 3, fetchIngredientRemove) -- SPELL_FETCH_SPIKY_SPIDER_EGG
RegisterAuraEvent(51072, 3, fetchIngredientRemove) -- SPELL_FETCH_HAIRY_HERRING_HEAD
RegisterAuraEvent(51077, 3, fetchIngredientRemove) -- SPELL_FETCH_PUTRID_PIRATE_PERSPIRATION
RegisterAuraEvent(51079, 3, fetchIngredientRemove) -- SPELL_FETCH_ICECROWN_BOTTLED_WATER
-- Medium
RegisterAuraEvent(51081, 3, fetchIngredientRemove) -- SPELL_FETCH_WASPS_WINGS
RegisterAuraEvent(51083, 3, fetchIngredientRemove) -- SPELL_FETCH_PRISMATIC_MOJO
RegisterAuraEvent(51085, 3, fetchIngredientRemove) -- SPELL_FETCH_RAPTOR_CLAW
RegisterAuraEvent(51087, 3, fetchIngredientRemove) -- SPELL_FETCH_AMBERSEED
RegisterAuraEvent(51091, 3, fetchIngredientRemove) -- SPELL_FETCH_SHRUNKEN_DRAGONS_CLAW
-- Hard
RegisterAuraEvent(51093, 3, fetchIngredientRemove) -- SPELL_FETCH_CHILLED_SERPENT_MUCUS
RegisterAuraEvent(51095, 3, fetchIngredientRemove) -- SPELL_FETCH_CRYSTALLIZED_HOGSNOT
RegisterAuraEvent(51097, 3, fetchIngredientRemove) -- SPELL_FETCH_CRUSHED_BASILISK_CRYSTALS
RegisterAuraEvent(51100, 3, fetchIngredientRemove) -- SPELL_FETCH_TROLLBANE
RegisterAuraEvent(51102, 3, fetchIngredientRemove) -- SPELL_FETCH_FROZEN_SPIDER_ICHOR

-- C++ spell_random_ingredient::HandleScriptEffect
-- (zone_zuldrak.cpp:726-747): OnEffectHitTarget, EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT. If the hit unit is a player, roll
-- the ingredient by own spell id (easy 0-10, medium 11-15, hard
-- 16-20) and the closest Finklestein (28205, 25yd) casts
-- FetchIngredients[ingredient][0] on the player, triggered, and
-- Talk(FetchIngredients[ingredient][3], player). The caster is
-- the player (aura-tick self-cast, row 524), so ev5 fires on the
-- player path (finishSpellCast -> fireSpellEffectHitTarget,
-- spells.go:14892). DORMANT: no Spell.dbc records for
-- 51134/51105/51107 yet.
-- FetchIngredients (zone_zuldrak.cpp:610-632), one table local:
-- {fetch, have, item, SAY} per row, in C++ order. SAY 5-25
-- (zone_zuldrak.cpp:427-456); the pot_check handler below reads
-- the have-aura ([2]) and item ([3]) columns; the random-ingredient
-- handler reads fetch ([1]) and SAY ([4]).
local FetchIngredients = {
    { 51018, 51047, 38338, 5 }, -- SPELL_FETCH_KNOTROOT / SAY_KNOTROOT
    { 51055, 51056, 38341, 6 }, -- SPELL_FETCH_PICKLED_EAGLE_EGG / SAY_PICKLED_EAGLE_EGG
    { 51057, 51058, 38337, 7 }, -- SPELL_FETCH_SPECKLED_GUANO / SAY_SPECKLED_GUANO
    { 51059, 51060, 38339, 8 }, -- SPELL_FETCH_WITHERED_BATWING / SAY_WITHERED_BATWING
    { 51062, 51063, 38381, 9 }, -- SPELL_FETCH_SEASONED_SLIDER_CIDER / SAY_SEASONED_SLIDER_CIDER
    { 51064, 51065, 38384, 10 }, -- SPELL_FETCH_PULVERIZED_GARGOYLE_TEETH / SAY_PULVERIZED_GARGOYLE_TEETH
    { 51067, 51068, 38386, 11 }, -- SPELL_FETCH_MUDDY_MIRE_MAGGOT / SAY_MUDDY_MIRE_MAGGOT
    { 51069, 51070, 38393, 12 }, -- SPELL_FETCH_SPIKY_SPIDER_EGG / SAY_SPIKY_SPIDER_EGG
    { 51072, 51075, 38396, 13 }, -- SPELL_FETCH_HAIRY_HERRING_HEAD / SAY_HAIRY_HERRING_HEAD
    { 51077, 51078, 38397, 14 }, -- SPELL_FETCH_PUTRID_PIRATE_PERSPIRATION / SAY_PUTRID_PIRATE_PERSPIRATION
    { 51079, 51080, 38398, 15 }, -- SPELL_FETCH_ICECROWN_BOTTLED_WATER / SAY_ICECROWN_BOTTLED_WATER
    { 51081, 51082, 38369, 16 }, -- SPELL_FETCH_WASPS_WINGS / SAY_WASPS_WINGS
    { 51083, 51084, 38343, 17 }, -- SPELL_FETCH_PRISMATIC_MOJO / SAY_PRISMATIC_MOJO
    { 51085, 51086, 38370, 18 }, -- SPELL_FETCH_RAPTOR_CLAW / SAY_RAPTOR_CLAW
    { 51087, 51088, 38340, 19 }, -- SPELL_FETCH_AMBERSEED / SAY_AMBERSEED
    { 51091, 51092, 38344, 20 }, -- SPELL_FETCH_SHRUNKEN_DRAGONS_CLAW / SAY_SHRUNKEN_DRAGONS_CLAW
    { 51093, 51094, 38346, 21 }, -- SPELL_FETCH_CHILLED_SERPENT_MUCUS / SAY_CHILLED_SERPENT_MUCUS
    { 51095, 51096, 38336, 22 }, -- SPELL_FETCH_CRYSTALLIZED_HOGSNOT / SAY_CRYSTALLIZED_HOGSNOT
    { 51097, 51098, 38379, 23 }, -- SPELL_FETCH_CRUSHED_BASILISK_CRYSTALS / SAY_CRUSHED_BASILISK_CRYSTALS
    { 51100, 51101, 38342, 24 }, -- SPELL_FETCH_TROLLBANE / SAY_TROLLBANE
    { 51102, 51104, 38345, 25 }, -- SPELL_FETCH_FROZEN_SPIDER_ICHOR / SAY_FROZEN_SPIDER_ICHOR
}

local function makeRandomIngredientHandler(lo, hi)
    return function(event, spell, target, effectIndex)
        if effectIndex ~= 0 then
            return
        end
        if target == nil or target:GetObjectType() ~= "Player" then
            return
        end
        local row = FetchIngredients[math.random(lo, hi) + 1]
        local finklestein = target:FindNearestCreature(NPC_FINKLESTEIN, 25.0)
        if finklestein == nil then
            return
        end
        finklestein:CastSpell(target, row[1], true)
        finklestein:Talk(row[4], target)
    end
end

RegisterSpellEvent(SPELL_RANDOM_INGREDIENT_EASY, 5, makeRandomIngredientHandler(0, 10))
RegisterSpellEvent(SPELL_RANDOM_INGREDIENT_MEDIUM, 5, makeRandomIngredientHandler(11, 15))
RegisterSpellEvent(SPELL_RANDOM_INGREDIENT_HARD, 5, makeRandomIngredientHandler(16, 20))

local SPELL_POT_CHECK = 51046 -- zone_zuldrak.cpp:375
local SPELL_THROW_INGREDIENT = 51025 -- zone_zuldrak.cpp:376
local SPELL_KILL_CREDIT = 51111 -- zone_zuldrak.cpp:377
local QUEST_THE_ALCHEMIST_APPRENTICE_DAILY = 12541 -- zone_zuldrak.cpp:365
local QUEST_STATUS_INCOMPLETE = 3 -- QuestDef.h:106

-- C++ spell_pot_check::HandleScriptEffect (zone_zuldrak.cpp:805-849):
-- OnEffectHitTarget, EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT. First-match
-- fetch aura (i 0-20, then return): self-cast 51025 + RemoveAura(fetch);
-- with the have aura also present, RemoveAura(have) + DestroyItemCount
-- (item, 1); i < 15 -> closest Finklestein (28205, 25yd) SetData(1, 1)
-- and return; i >= 15 + quest 12541 INCOMPLETE -> RemoveAura(51216) +
-- self-cast 51111, and the loop CONTINUES (no return on that leg, as in
-- C++). Without the have aura: destroy 1 of every ingredient item the
-- player holds + RemoveAura(51216) + Finklestein Talk(4, player) + return.
-- The caster is the player (cauldron gossip self-cast, :595-599), so ev5
-- fires on the player path (finishSpellCast -> fireSpellEffectHitTarget,
-- spells.go:14892). DORMANT: no Spell.dbc 51046 record, and the cauldron
-- GO gossip that casts it is registration-only blocked.
local function potCheckHit(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local player = hitTarget
    if player == nil or player:GetObjectType() ~= "Player" then
        return
    end
    for i = 1, 21 do
        local row = FetchIngredients[i]
        if player:HasAura(row[1]) then
            player:CastSpell(player, SPELL_THROW_INGREDIENT)
            player:RemoveAura(row[1])
            if player:HasAura(row[2]) then
                player:RemoveAura(row[2])
                player:DestroyItemCount(row[3], 1, true)
                if i <= 15 then
                    local finklestein = player:FindNearestCreature(NPC_FINKLESTEIN, 25.0)
                    if finklestein ~= nil then
                        finklestein:SetData(1, 1)
                    end
                    return
                elseif player:GetQuestStatus(QUEST_THE_ALCHEMIST_APPRENTICE_DAILY) == QUEST_STATUS_INCOMPLETE then
                    player:RemoveAura(SPELL_ALCHEMIST_APPRENTICE_INVISBUFF)
                    player:CastSpell(player, SPELL_KILL_CREDIT)
                end
            else
                for j = 1, 21 do
                    if player:HasItem(FetchIngredients[j][3]) then
                        player:DestroyItemCount(FetchIngredients[j][3], 1, true)
                    end
                end
                player:RemoveAura(SPELL_ALCHEMIST_APPRENTICE_INVISBUFF)
                local finklestein = player:FindNearestCreature(NPC_FINKLESTEIN, 25.0)
                if finklestein ~= nil then
                    finklestein:Talk(SAY_RUINED, player)
                end
                return
            end
        end
    end
end

RegisterSpellEvent(SPELL_POT_CHECK, 5, potCheckHit)

-- PORTED (hook strip 19, RegisterCreatureEvent ev14 = OnHitBySpell /
-- ev23 = OnReset / ev28 = MovementInform / ev41 = OnSetData +
-- RegisterCreatureGossipEvent select = 2):
--   npc_drakuru_shackles (29700, zone_zuldrak.cpp:47-127): Reset
--   summons the Captured Rageclaw (29686) at GetClosePoint and
--   faces both to each other; SpellHit (Unlock Shackle 55083,
--   player caster, quest 12861 incomplete) frees it — DoCast Free
--   Rageclaw 55223 on the rageclaw, KilledMonsterCredit 29686
--   (the Player::KilledMonster quest-credit twin, colonel_jules
--   precedent), RemoveAura Chain Right 54990, DespawnOrUnsummon;
--   rageclaw not found -> SetDeathState JUST_DIED (1).
--   npc_captured_rageclaw (29686, :129-170): Reset sets faction
--   friendly + Chain Right self-cast; SpellHit (Free Rageclaw
--   55223) removes Chain Left (55009), stands, restores the
--   template faction (2117, creature_template.sql), casts
--   Unshackled (55085), talks, wanders off (MoveRandom 10) and
--   despawns in 10s.
--   npc_storm_cloud (29939, :901-961 — STORM_COULD): Reset
--   self-casts Storm Visual (55708). SpellHit (Gymer's Grab
--   55516 -> Ride Vehicle 43671 + Healing Winds 55549 on the
--   caster's vehicle) STAYS OUT: the seat-count gate
--   (GetAvailableSeatCount) has no bridge on the Lua vehicle
--   object (lua_vehicle.go:62-115 — GetObjectType / GetOwner /
--   GetEntry / GetPassenger / IsOnBoard / AddPassenger /
--   RemovePassenger only).
--   npc_released_offspring_harkoa (28526, :172-217): Reset walks
--   to a close point (MoveTo id 0); MovementInform (ev28, POINT
--   type 8) despawns on arrival.
--   npc_crusade_recruit (28090, :219-303): Reset sets the gossip
--   flag + cower emotestate and records the spawn heading for the
--   unported UpdateAI/gossip machine (ev7 + gossip scope, not
--   this row).
--   npc_alchemist_finklestein (28205, :468-587): full AI port —
--   Reset clears the player/try state and starts the turn-to-pot
--   cycle (15-26s); turn legs face the pot / back with the
--   matching emotes; gossip select casts the invisbuff (51216)
--   and starts the ingredient chain; SetData (ev41, the
--   pot_check call site above) schedules the try-gated
--   ingredient event.
--   go_scourge_enclosure / go_finklesteins_cauldron are
--   GameObjectScripts — no creature ev23 applies.
--   All ev23 handlers below are IsAlive-gated (attumen /
--   broodlord precedent — Go fires ev23 on the death path too).
local ENTRY_DRAKURU_SHACKLES = 29700 -- creature_template.sql (name 'Drakuru Shackles')
local NPC_RAGECLAW = 29686 -- zone_zuldrak.cpp:38
local QUEST_TROLLS_IS_GONE_CRAZY = 12861 -- zone_zuldrak.cpp:39
local SPELL_CHAIN_OF_THE_SCURGE_RIGHT = 54990 -- zone_zuldrak.cpp:40
local SPELL_CHAIN_OF_THE_SCURGE_LEFT = 55009 -- zone_zuldrak.cpp:41
local SPELL_UNLOCK_SHACKLE = 55083 -- zone_zuldrak.cpp:42
local SPELL_FREE_RAGECLAW = 55223 -- zone_zuldrak.cpp:43
local SPELL_UNSHACKLED = 55085 -- zone_zuldrak.cpp:131
local SAY_RAGECLAW = 0 -- zone_zuldrak.cpp:132
local ENTRY_STORM_CLOUD = 29939 -- zone_zuldrak.cpp:903 (STORM_COULD)
local SPELL_STORM_VISUAL = 55708 -- zone_zuldrak.cpp:905
local ENTRY_RELEASED_OFFSPRING_HARKOA = 28526 -- creature_template.sql (name 'Released Offspring of Har'koa')
local ENTRY_CRUSADE_RECRUIT = 28090 -- creature_template.sql (name 'Crusade Recruit')
local UNIT_FIELD_FLAGS = 59 -- UpdateFields.h (curator precedent)
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- SharedDefines.h (curator precedent)
local UNIT_NPC_FLAGS = 82 -- UpdateFields.h:139 (nefarian precedent)
local UNIT_NPC_FLAG_GOSSIP = 0x00000001 -- UnitDefines.h:186
local UNIT_NPC_EMOTESTATE = 83 -- UpdateFields.h:140 (OBJECT_END + 0x4D)
local EMOTE_STATE_COWER = 431 -- SharedDefines.h:2068
local EMOTE_STATE_NONE = 30 -- SharedDefines.h:1965
local EMOTE_STATE_USE_STANDING_NO_SHEATHE = 133 -- SharedDefines.h:1994
local FACTION_FRIENDLY = 35 -- SharedDefines.h:231
local TEMPSUMMON_DEAD_DESPAWN = 7 -- ObjectDefines.h:65
local POINT_MOTION_TYPE = 8 -- MovementDefines.h:36 (flamecaller precedent)
local SAY_EASY_123 = 0 -- zone_zuldrak.cpp:425
local SAY_MEDIUM_4 = 1 -- zone_zuldrak.cpp:426
local SAY_MEDIUM_5 = 2 -- zone_zuldrak.cpp:427
local SAY_HARD_6 = 3 -- zone_zuldrak.cpp:428

local shacklesState = {}

local function shacklesReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    local reach = creature:GetCombatReach()
    local x, y, z = creature:GetRelativePoint(reach / 3 + 0.1, 0) -- GetClosePoint(reach/3, 0.1): total reach + reach/3 + 0.1 (king_dred precedent)
    local rageclaw = creature:SummonCreature(NPC_RAGECLAW, x, y, z, 0, TEMPSUMMON_DEAD_DESPAWN, 1000)
    if rageclaw ~= nil then
        shacklesState[creature:GetGUIDLow()] = { rageclawGUID = rageclaw:GetGUIDLow() }
        creature:SetFacingToObject(rageclaw)
        rageclaw:SetFacingToObject(creature)
    end
end

local function shacklesSpellHit(event, creature, caster, spellId)
    if caster == nil or caster:GetObjectType() ~= "Player" then
        return
    end
    if spellId ~= SPELL_UNLOCK_SHACKLE then
        return
    end
    if caster:GetQuestStatus(QUEST_TROLLS_IS_GONE_CRAZY) ~= QUEST_STATUS_INCOMPLETE then
        return
    end
    local state = shacklesState[creature:GetGUIDLow()]
    local rageclaw = state ~= nil and creature:GetCreatureByGUID(state.rageclawGUID) or nil
    if rageclaw ~= nil then
        rageclaw:CastSpell(rageclaw, SPELL_FREE_RAGECLAW, true)
        creature:SetDeathState(3) -- UnlockRageclaw :80 setDeathState(DEAD)
        caster:KilledMonsterCredit(NPC_RAGECLAW)
        creature:RemoveAura(SPELL_CHAIN_OF_THE_SCURGE_RIGHT)
        creature:DespawnOrUnsummon()
    else
        creature:SetDeathState(1) -- JUST_DIED (Unit.h:200-207)
    end
end

local function rageclawReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    creature:SetFaction(FACTION_FRIENDLY)
    creature:CastSpell(creature, SPELL_CHAIN_OF_THE_SCURGE_RIGHT, true)
end

local function rageclawSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_FREE_RAGECLAW then
        return
    end
    creature:RemoveAura(SPELL_CHAIN_OF_THE_SCURGE_LEFT)
    creature:SetStandState(0) -- UNIT_STAND_STATE_STAND (UnitDefines.h:33)
    creature:SetFaction(2117) -- template faction, creature_template.sql row 29686
    creature:CastSpell(creature, SPELL_UNSHACKLED, true)
    creature:Talk(SAY_RAGECLAW)
    creature:MoveRandom(10)
    creature:DespawnOrUnsummon(10000)
end

local function stormCloudReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    creature:CastSpell(creature, SPELL_STORM_VISUAL, true)
end

local function harkoaReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    local reach = creature:GetCombatReach()
    local x, y, z = creature:GetRelativePoint(reach / 3 + 25.0, 0) -- GetClosePoint(reach/3, 25.0): total reach + reach/3 + 25.0 (king_dred precedent)
    creature:MoveTo(0, x, y, z)
end

local function harkoaMovementInform(event, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE then
        return
    end
    creature:DespawnOrUnsummon()
end

local recruitState = {}

local function crusadeRecruitReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    creature:SetFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP)
    creature:SetUInt32Value(UNIT_NPC_EMOTESTATE, EMOTE_STATE_COWER)
    recruitState[creature:GetGUIDLow()] = { heading = creature:GetO() }
end

local finklesteinState = {}
local finklesteinTimers = {}

local function cancelFinklesteinTimers(guid)
    local per = finklesteinTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        finklesteinTimers[guid] = nil
    end
end

local function scheduleFinklestein(guid, key, delay, fn)
    local per = finklesteinTimers[guid]
    if not per then
        per = {}
        finklesteinTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local finklesteinTurnToPot, finklesteinTurnBack

finklesteinTurnBack = function(creature, guid)
    creature:SetFacingTo(4.886922)
    creature:SetUInt32Value(UNIT_NPC_EMOTESTATE, EMOTE_STATE_NONE)
    scheduleFinklestein(guid, "turntopot", {25000, 41000}, function()
        finklesteinTurnToPot(creature, guid)
    end)
end

finklesteinTurnToPot = function(creature, guid)
    creature:SetFacingTo(6.230825)
    creature:SetUInt32Value(UNIT_NPC_EMOTESTATE, EMOTE_STATE_USE_STANDING_NO_SHEATHE)
    scheduleFinklestein(guid, "turnback", 11000, function()
        finklesteinTurnBack(creature, guid)
    end)
end

local function finklesteinReset(event, creature)
    if not creature:IsAlive() then
        return
    end
    local guid = creature:GetGUIDLow()
    cancelFinklesteinTimers(guid)
    finklesteinState[guid] = { playerGUID = nil, tryCount = 0 }
    scheduleFinklestein(guid, "turntopot", {15000, 26000}, function()
        finklesteinTurnToPot(creature, guid)
    end)
end

local function finklesteinIngredient(creature, guid, say, aura)
    local state = finklesteinState[guid]
    if state == nil or state.playerGUID == nil then
        return
    end
    local player = GetPlayerByGUID(state.playerGUID)
    if player ~= nil then
        creature:Talk(say, player)
        creature:CastSpell(player, aura)
        state.tryCount = state.tryCount + 1
    end
end

local function finklesteinGossipSelect(event, player, creature, sender, action, code, listId)
    player:GossipClearMenu()
    player:GossipComplete()
    creature:CastSpell(player, SPELL_ALCHEMIST_APPRENTICE_INVISBUFF)
    local guid = creature:GetGUIDLow()
    local state = finklesteinState[guid]
    if state == nil then
        state = { playerGUID = nil, tryCount = 0 }
        finklesteinState[guid] = state
    end
    state.playerGUID = player:GetGUIDLow()
    state.tryCount = 1
    scheduleFinklestein(guid, "ingredient", 100, function()
        finklesteinIngredient(creature, guid, SAY_EASY_123, SPELL_RANDOM_INGREDIENT_EASY_AURA)
    end)
    return false
end

local function finklesteinSetData(event, creature, type, data)
    if type ~= 1 or data ~= 1 then
        return
    end
    local guid = creature:GetGUIDLow()
    local state = finklesteinState[guid]
    if state == nil then
        return
    end
    if state.tryCount == 2 or state.tryCount == 3 then
        scheduleFinklestein(guid, "ingredient", 100, function()
            finklesteinIngredient(creature, guid, SAY_EASY_123, SPELL_RANDOM_INGREDIENT_EASY_AURA)
        end)
    elseif state.tryCount == 4 then
        scheduleFinklestein(guid, "ingredient", 100, function()
            finklesteinIngredient(creature, guid, SAY_MEDIUM_4, SPELL_RANDOM_INGREDIENT_MEDIUM_AURA)
        end)
    elseif state.tryCount == 5 then
        scheduleFinklestein(guid, "ingredient", 100, function()
            finklesteinIngredient(creature, guid, SAY_MEDIUM_5, SPELL_RANDOM_INGREDIENT_MEDIUM_AURA)
        end)
    elseif state.tryCount == 6 then
        scheduleFinklestein(guid, "ingredient", 100, function()
            finklesteinIngredient(creature, guid, SAY_HARD_6, SPELL_RANDOM_INGREDIENT_HARD_AURA)
        end)
    end
end

RegisterCreatureEvent(ENTRY_DRAKURU_SHACKLES, 23, shacklesReset)
RegisterCreatureEvent(ENTRY_DRAKURU_SHACKLES, 14, shacklesSpellHit)
RegisterCreatureEvent(NPC_RAGECLAW, 23, rageclawReset)
RegisterCreatureEvent(NPC_RAGECLAW, 14, rageclawSpellHit)
RegisterCreatureEvent(ENTRY_STORM_CLOUD, 23, stormCloudReset)
RegisterCreatureEvent(ENTRY_RELEASED_OFFSPRING_HARKOA, 23, harkoaReset)
RegisterCreatureEvent(ENTRY_RELEASED_OFFSPRING_HARKOA, 28, harkoaMovementInform)
RegisterCreatureEvent(ENTRY_CRUSADE_RECRUIT, 23, crusadeRecruitReset)
RegisterCreatureEvent(NPC_FINKLESTEIN, 23, finklesteinReset)
RegisterCreatureEvent(NPC_FINKLESTEIN, 41, finklesteinSetData)
RegisterCreatureGossipEvent(NPC_FINKLESTEIN, 2, finklesteinGossipSelect)
