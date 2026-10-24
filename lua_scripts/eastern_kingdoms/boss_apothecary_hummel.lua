-- Apothecary Hummel (Shadowfang Keep) -- Lua port of
-- src/server/scripts/EasternKingdoms/ShadowfangKeep/
-- boss_apothecary_hummel.cpp (boss_apothecary_hummelAI : public BossAI
-- via GetShadowfangKeepAI; registered by AddSC_boss_apothecary_hummel in
-- eastern_kingdoms_script_loader.cpp (declaration line 125, call line 303)).
-- The file holds 9 scripts: boss_apothecary_hummel, npc_apothecary_baxter
-- and npc_apothecary_frye (ported in npc_apothecary_baxter.lua /
-- npc_apothecary_frye.lua), and six SpellScript/AuraScript loaders —
-- spell_apothecary_lingering_fumes (68965), spell_apothecary_validate_area
-- (68644), spell_apothecary_throw_cologne (69038), spell_apothecary_throw_
-- perfume (68966), spell_apothecary_perfume_spill (68798, AuraScript),
-- spell_apothecary_cologne_spill (68614, AuraScript) — all documented-only
-- (no SpellScript/AuraScript bridges, nightbane precedent).
-- Entry: no NPC_ constant in the C++ tree (DB-side ScriptName binding) —
-- 36296 independently cited (wowhead npc=36296, TrinityCore issue #16629).
-- DATA_APOTHECARY_HUMMEL=6 / DATA_SPAWN_VALENTINE_ADDS=7 per
-- shadowfang_keep.h:33-34.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (pre-damage hook), 23 OnReset, 34 OnQuestReward (fire
-- site exists in quest_reward_grant.go). Gossip events: 1 OnGossipHello,
-- 2 OnGossipSelect (fires inline in gossip.go, coren direbrew precedent).
-- Timers via CreateLuaEvent; melee is engine-driven (DoMeleeAttackIfReady).
-- Ported arms (C++-exact where bridges exist):
-- - OnGossipHello: C++ has no handler (menu 10847 is DB-side); the fight
--   is otherwise unreachable, so one approximated start option is offered
--   (coren/vael precedent — the real text lives in broadcast_text).
-- - OnGossipSelect: C++ checks menuId==GOSSIP_MENU_HUMMEL(10847) &&
--   gossipListId==GOSSIP_OPTION_START(0) — neither reaches the Go hook,
--   which carries only sender/action, so sender==MAIN && action==DEF+1 is
--   the start option (documented): GossipClearMenu + GossipComplete
--   (CloseGossipMenuFor) + DoAction(ACTION_START_EVENT) when in PHASE_ALL.
-- - OnQuestReward: quest.ID==QUEST_YOUVE_BEEN_SERVED(14488) ->
--   DoAction(ACTION_START_EVENT) when in PHASE_ALL (Eluna arg order is
--   (event, player, creature, quest), lua_creature_events.go:458).
-- - Intro machine (ACTION_START_EVENT, PHASE_ALL->PHASE_INTRO):
--   EVENT_HUMMEL_SAY_0 at 1ms -> Talk(0) -> SAY_1 at 4s -> Talk(1) ->
--   SAY_2 at 4s -> Talk(2) -> START_FIGHT at 4s (C++ re-arms from each
--   execution; one-shot Lua timer chains match).
-- - EVENT_START_FIGHT: schedule EVENT_CALL_BAXTER 6s -> Talk(3),
--   EVENT_CALL_FRYE 14s -> Talk(4), EVENT_PERFUME_SPRAY 3640ms,
--   EVENT_CHAIN_REACTION 15s, EVENT_CALL_CRAZED_APOTHECARY 15s ->
--   Talk(6). EVENT_CRAZED_APOTHECARY's only arm is the unbridgeable
--   instance SetData(DATA_SPAWN_VALENTINE_ADDS, 0), so no timer is kept
--   (documented-only).
-- - EVENT_PERFUME_SPRAY: DoCastVictim(SPELL_PERFUME_SPRAY 68607)
--   non-triggered -> GetVictim + CastSpell (jeklik convention, nil-victim
--   keeps schedule); 3640ms re-arm.
-- - EVENT_CHAIN_REACTION: DoCastVictim(SPELL_SUMMON_TABLE 69218, triggered)
--   -> CastSpell(victim, 69218, true); the DoCastAOE(SPELL_CHAIN_REACTION
--   68821) leg is a non-triggered self-cast (selin_fireheart DoCastAOE
--   convention — the old "no AoE-cast bridge" claim was wrong); 25s
--   re-arm. The summon-table leg's actual summon has no bridge, so no
--   table spawns (documented).
-- - DamageTaken: C++ `damage >= me->GetHealth()` is the lethal-blow class
--   (darkreaver precedent, NOT the golemagg HealthBelowPct bug class):
--   if _deadCount < 2, clamp the blow to health-1 via the event-9 second
--   return (opera precedent) and, once, Talk(SAY_HUMMEL_DEATH 5) +
--   triggered self-cast SPELL_PERMANENT_FEIGN_DEATH 29266. Per-GUID latch
--   cleared on Reset (2/4/23 cancel timers, maiden convention).
-- - JustDied: Talk(5) only when the feign-death latch never fired (C++
--   `if (!_isDead)`), then cancel timers.
-- Deviation from C++ (documented): the intro is gated on gossip/quest in
-- C++; SetFaction and the unit-flags immunity surface are bridged now,
-- but the intro gate itself stays gossip/quest-driven, so a direct pull
-- also starts the intro via OnEnterCombat when still in PHASE_ALL (coren
-- precedent: the fight starts when a player pulls).
-- Unmodeled (documented-only, no bridges):
-- - Reset: SummonCreatureGroup(1) only (Baxter/Frye spawn from the group;
--   no group-summon bridge; the SetFaction(FACTION_FRIENDLY) leg is PORTED
--   in onReset, and the per-GUID latch is cleared in Lua).
-- - EnterEvadeMode: summons.DespawnAll + _DespawnAtEvade(10s) — no
--   summon/evade bridges; Lua cancels timers on 2.
-- - DoAction: SetImmuneToPC(true) + SetFaction(FACTION_MONSTER) +
--   summons.DoAction(ACTION_START_EVENT, DummyEntryCheckPredicate) relay —
--   the immunity/faction bridges are live (unit-flags surface), but the
--   arm stays out behind the unported DoAction/summon-relay machine.
-- - DamageTaken SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_UNK_29 |
--   UNIT_FLAG_NOT_SELECTABLE) leg — PORTED (2026-10-20); the
--   RemoveAurasDueToSpell(SPELL_ALLURING_PERFUME 68589) leg is PORTED via
--   creature:RemoveAura (stale "no aura-remove bridge" claim retracted).
-- - SummonedCreatureDies: the event-21 dispatch IS bridged now
--   (2026-10-24, retiring the old "no fire site" claim), but the
--   baxter/frye summon group (Reset SummonCreatureGroup(1)) is unported,
--   so the arm has no bearer — _deadCount never increments in the port,
--   the lethal clamp persists, and the DoCastSelf(SPELL_QUIET_SUICIDE
--   3617) arm that ends the feign-death in C++ cannot fire (the
--   mandokir/nefarian no-bearer precedent).
-- - JustDied: RemoveFlag + instance->SetBossState(DATA_APOTHECARY_HUMMEL,
--   DONE) + sLFGMgr->FinishDungeon(288) — no instance/LFG bridges.
-- - START_FIGHT: the SetImmuneToAll(false) leg is PORTED (2026-10-23) as
--   RemoveFlag(59, 0x300) in onStartFight (the keepCombat=false combat-end
--   leg drops, no bridge); the DoZoneInCombat (:220) leg IS PORTED
--   (2026-10-23 — plain me-call in C++ order after RemoveFlag, bridge
--   live); the despawn NPC_CROWN_APOTHECARY 36885 in grid sibling stays
--   out (unbridged grid search).
-- - CALL_BAXTER/FRYE: summons.DoAction(ACTION_START_FIGHT,
--   EntryCheckPredicate) + summons.DoZoneInCombat(BAXTER) relay — no
--   bridges (2026-10-23 re-evaluation: the relay is a summons-container
--   call on a DIFFERENT creature riding the unported DoAction machine;
--   stays out); the Talk arms are modeled.
-- - npc_apothecary_genericAI DoAction legs (MovePoint to BaxterMovePos /
--   FryeMovePos + EMOTE_STATE_USE_STANDING on MovementInform;
--   ACTION_START_FIGHT immunity/combat legs) — no MotionMaster/
--   MovementInform bridges; the emote-state and immunity surfaces are
--   bridged, but the legs ride the unported DoAction machine (already
--   noted in the baxter/frye files).
-- - UpdateAI UNIT_STATE_CASTING gates — no bridge (maiden precedent).
-- Verifiable numbers (file's own enums): SAY_INTRO_0..2 = 0..2,
-- SAY_CALL_BAXTER = 3, SAY_CALL_FRYE = 4, SAY_HUMMEL_DEATH = 5,
-- SAY_SUMMON_ADDS = 6; SPELL_ALLURING_PERFUME = 68589,
-- SPELL_PERFUME_SPRAY = 68607, SPELL_CHAIN_REACTION = 68821,
-- SPELL_SUMMON_TABLE = 69218, SPELL_PERMANENT_FEIGN_DEATH = 29266,
-- SPELL_QUIET_SUICIDE = 3617, SPELL_COLOGNE_SPRAY = 68948,
-- NPC_APOTHECARY_FRYE = 36272, NPC_APOTHECARY_BAXTER = 36565,
-- NPC_CROWN_APOTHECARY = 36885; QUEST_YOUVE_BEEN_SERVED = 14488,
-- GOSSIP_MENU_HUMMEL = 10847.

local ENTRY_HUMMEL = 36296

-- UnitDefines.h:149/153; SetFlag/RemoveFlag on UNIT_FIELD_FLAGS (59)
-- is bridged (engine/world/lua_creature_events.go:968).
local UNIT_FIELD_FLAGS = 59
local FACTION_FRIENDLY = 35
local UNIT_FLAG_UNK_29 = 0x20000000
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

local SAY_INTRO_0 = 0
local SAY_INTRO_1 = 1
local SAY_INTRO_2 = 2
local SAY_CALL_BAXTER = 3
local SAY_CALL_FRYE = 4
local SAY_HUMMEL_DEATH = 5
local SAY_SUMMON_ADDS = 6

local SPELL_PERFUME_SPRAY = 68607
local SPELL_SUMMON_TABLE = 69218
local SPELL_PERMANENT_FEIGN_DEATH = 29266
local SPELL_ALLURING_PERFUME = 68589
local SPELL_CHAIN_REACTION = 68821

local QUEST_YOUVE_BEEN_SERVED = 14488

local GOSSIP_SENDER_MAIN = 1
local GOSSIP_ACTION_INFO_DEF = 1000
local GOSSIP_START_TEXT = "We are ready."

local PHASE_ALL = 0
local PHASE_INTRO = 1

local timers = {}
local state = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

local function clearState(guid)
    cancelTimers(guid)
    state[guid] = nil
end

local function schedule(guid, key, delay, fn)
    local per = timers[guid]
    if not per then
        per = {}
        timers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function getState(guid)
    local st = state[guid]
    if not st then
        st = { phase = PHASE_ALL, deadCount = 0, isDead = false }
        state[guid] = st
    end
    return st
end

-- C++ EVENT_CHAIN_REACTION: triggered DoCastVictim(SPELL_SUMMON_TABLE)
-- + non-triggered DoCastAOE(SPELL_CHAIN_REACTION) (self-cast,
-- selin_fireheart DoCastAOE convention — the old "unbridgeable" claim
-- was wrong); 25s re-arm. SPELL_SUMMON_TABLE's summon leg has no
-- summon bridge, so the table never appears (documented).
local function onChainReaction(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SUMMON_TABLE, true)
    end
    creature:CastSpell(creature, SPELL_CHAIN_REACTION)
    schedule(guid, "chain", 25000, function() onChainReaction(creature, guid) end)
end

-- C++ EVENT_PERFUME_SPRAY: non-triggered DoCastVictim(68607); 3640ms.
local function onPerfumeSpray(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_PERFUME_SPRAY)
    end
    schedule(guid, "perfume", 3640, function() onPerfumeSpray(creature, guid) end)
end

-- C++ EVENT_CALL_CRAZED_APOTHECARY: Talk(SAY_SUMMON_ADDS) only.
local function onCallCrazed(creature, guid)
    creature:Talk(SAY_SUMMON_ADDS)
end

-- C++ EVENT_CALL_FRYE: Talk(SAY_CALL_FRYE); the DoAction relay has no
-- bridge.
local function onCallFrye(creature, guid)
    creature:Talk(SAY_CALL_FRYE)
end

-- C++ EVENT_CALL_BAXTER: Talk(SAY_CALL_BAXTER); the DoAction/DoZoneInCombat
-- summon relays stay out — summons-container call on a different
-- creature riding the unported DoAction machine (2026-10-23 re-eval).
local function onCallBaxter(creature, guid)
    creature:Talk(SAY_CALL_BAXTER)
end

-- C++ EVENT_START_FIGHT: arms the combat pumps. The SetImmuneToAll(false)
-- leg (boss_apothecary_hummel.cpp:219) ports first, in C++ arm order, as
-- the flag twin RemoveFlag(UNIT_FIELD_FLAGS 59, 0x100|0x200 = 0x300)
-- (Unit.cpp:8440-8451; SetImmuneToPC-sweep precedent): the keepCombat=false
-- combat-end leg has no bridge (dropped); the DoZoneInCombat() arm
-- :220 IS PORTED below (C++ order, bridge live 2026-10-23); the
-- NPC_CROWN_APOTHECARY grid-despawn sibling stays out (no
-- GetCreatureListWithEntryInGrid bridge, 2026-10-23 re-eval).
local function onStartFight(creature, guid)
    creature:RemoveFlag(59, 0x300)
    creature:DoZoneInCombat() -- boss_apothecary_hummel.cpp:220 (EVENT_START_FIGHT arm 2, C++ order after SetImmuneToAll(false); bridge live 2026-10-23)
    schedule(guid, "baxter", 6000, function() onCallBaxter(creature, guid) end)
    schedule(guid, "frye", 14000, function() onCallFrye(creature, guid) end)
    schedule(guid, "perfume", 3640, function() onPerfumeSpray(creature, guid) end)
    schedule(guid, "chain", 15000, function() onChainReaction(creature, guid) end)
    schedule(guid, "crazed", 15000, function() onCallCrazed(creature, guid) end)
end

-- C++ EVENT_HUMMEL_SAY_2 -> Talk(2) -> EVENT_START_FIGHT in 4s.
local function onSay2(creature, guid)
    creature:Talk(SAY_INTRO_2)
    schedule(guid, "say", 4000, function() onStartFight(creature, guid) end)
end

-- C++ EVENT_HUMMEL_SAY_1 -> Talk(1) -> SAY_2 in 4s.
local function onSay1(creature, guid)
    creature:Talk(SAY_INTRO_1)
    schedule(guid, "say", 4000, function() onSay2(creature, guid) end)
end

-- C++ DoAction(ACTION_START_EVENT): PHASE_ALL -> PHASE_INTRO, SAY_0 at 1ms.
local function startEvent(creature)
    local guid = creature:GetGUID()
    local st = getState(guid)
    if st.phase ~= PHASE_ALL then
        return
    end
    st.phase = PHASE_INTRO
    schedule(guid, "say", 1, function()
        creature:Talk(SAY_INTRO_0)
        schedule(guid, "say", 4000, function() onSay1(creature, guid) end)
    end)
end

local function onGossipHello(event, player, creature)
    player:GossipMenuAddItem(0, GOSSIP_START_TEXT, GOSSIP_SENDER_MAIN, GOSSIP_ACTION_INFO_DEF + 1)
    player:GossipSendMenu(0, creature)
end

local function onGossipSelect(event, player, creature, sender, action)
    if sender ~= GOSSIP_SENDER_MAIN then
        return
    end
    if action == GOSSIP_ACTION_INFO_DEF + 1 then
        player:GossipClearMenu()
        player:GossipComplete()
        -- C++ OnGossipSelect: me->RemoveFlag(UNIT_NPC_FLAGS,
        -- UNIT_NPC_FLAG_GOSSIP) (boss_apothecary_hummel.cpp:109) — the
        -- start option goes away once the event begins. RemoveFlag(82, 1)
        -- is bridged (lua_creature_events.go:1086); field 82 =
        -- UNIT_NPC_FLAGS (UpdateFields.h:139), flag 1 = GOSSIP.
        creature:RemoveFlag(82, 1)
        startEvent(creature)
    end
    -- C++ OnGossipSelect returns false: the AI/native select arms
    -- proceed (Eluna GOSSIP_EVENT_ON_SELECT: a Lua false lets them
    -- proceed; true/nothing would veto them).
    return false
end

local function onQuestReward(event, player, creature, quest)
    if quest and quest.ID == QUEST_YOUVE_BEEN_SERVED then
        startEvent(creature)
    end
end

local function onEnterCombat(event, creature)
    startEvent(creature)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ EnterEvadeMode: summons.DespawnAll() (moot — nothing modeled)
    -- + _EnterEvadeMode() + _DespawnAtEvade(Seconds(10))
    -- (boss_apothecary_hummel.cpp:124-130); the 10s is respawn-side, the
    -- despawn itself is immediate (ScriptedCreature.cpp:616).
    creature:Despawn()
end

local function onDied(event, creature)
    local guid = creature:GetGUID()
    local st = state[guid]
    if not st or not st.isDead then
        creature:Talk(SAY_HUMMEL_DEATH)
    end
    clearState(guid)
    -- C++ JustDied (boss_apothecary_hummel.cpp:179), in C++ order after
    -- events.Reset: RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_UNK_29 |
    -- UNIT_FLAG_NOT_SELECTABLE) = RemoveFlag(59, 0x20000000|0x2000000);
    -- the instance->SetBossState/LFG legs have no bridges.
    creature:RemoveFlag(59, 0x22000000)
end

local function onReset(event, creature)
    clearState(creature:GetGUID())
    -- C++ Reset (boss_apothecary_hummel.cpp:121): SetFaction(FACTION_FRIENDLY);
    -- the SummonCreatureGroup(1) leg stays out (no group-summon bridge).
    creature:SetFaction(FACTION_FRIENDLY)
end

-- C++ DamageTaken: lethal-blow class (damage >= health, pre-damage) ->
-- clamp to health-1 while _deadCount < 2; once, Talk(5) + triggered
-- self-cast SPELL_PERMANENT_FEIGN_DEATH. The second return rewrites the
-- damage (opera precedent); nothing is returned on non-lethal hits
-- (thekal convention).
local function onDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = getState(guid)
    if damage >= creature:GetHealth() and st.deadCount < 2 then
        if not st.isDead then
            st.isDead = true
            -- C++-exact arm order: RemoveAurasDueToSpell(SPELL_ALLURING_PERFUME)
            -- via creature:RemoveAura (the "no aura-remove bridge" claim was
            -- stale — lua_objects.go), then triggered self-cast
            -- SPELL_PERMANENT_FEIGN_DEATH; the SetFlag(UNIT_FIELD_FLAGS,
            -- UNK_29|NOT_SELECTABLE) leg is PORTED (2026-10-20);
            -- Talk(SAY_HUMMEL_DEATH) last.
            creature:RemoveAura(SPELL_ALLURING_PERFUME)
            creature:CastSpell(creature, SPELL_PERMANENT_FEIGN_DEATH, true)
            -- boss_apothecary_hummel.cpp:158, C++ arm order
            creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_UNK_29 + UNIT_FLAG_NOT_SELECTABLE)
            creature:Talk(SAY_HUMMEL_DEATH)
        end
        return false, creature:GetHealth() - 1
    end
end

RegisterCreatureEvent(ENTRY_HUMMEL, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_HUMMEL, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY_HUMMEL, 4, onDied)
RegisterCreatureEvent(ENTRY_HUMMEL, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_HUMMEL, 23, onReset)
RegisterCreatureEvent(ENTRY_HUMMEL, 34, onQuestReward)
RegisterCreatureGossipEvent(ENTRY_HUMMEL, 1, onGossipHello)
RegisterCreatureGossipEvent(ENTRY_HUMMEL, 2, onGossipSelect)
