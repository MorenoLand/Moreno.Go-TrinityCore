-- Grizzly Hills: npc_lake_frog — Lua port of
-- src/server/scripts/Northrend/zone_grizzly_hills.cpp
-- (npc_lake_frog : public CreatureScript; registered in AddSC_zone_grizzly_hills()).
-- Entries: NPC_LAKE_FROG = 33211 / NPC_LAKE_FROG_QUEST = 33224
-- (zone_grizzly_hills.cpp:655-667 LakeFrog enum). The C++ tree has a single
-- npc_lake_frog script (one `new` at :1130), so both entries share this AI —
-- both are registered.
-- Ported: Reset's RemoveFlag(UNIT_NPC_FLAGS, UNIT_NPC_FLAG_GOSSIP) twin on
-- spawn (event 5) — C++ Reset (:702-707) is entry-conditional: it removes
-- the flag ONLY for NPC_LAKE_FROG_QUEST (33224), so only that entry is
-- registered below. 33211 keeps its template flag exactly like C++.
-- The select arm below stays gated exactly like C++ —
-- plus the OnGossipSelect DoCast(player, SPELL_SUMMON_ASHWOOD_BRAND 62554)
-- arm (:774-778), non-triggered (default DoCast overload); return false lets
-- the AI/native select arms proceed (Eluna gossip polarity, gossip.go).
-- Ported: the ReceiveEmote kiss machine (zone_grizzly_hills.cpp:746-777,
-- npc_lake_frogAI::ReceiveEmote, ev8 — the trigger is the emote, not
-- MoveInLineOfSight): TEXT_EMOTE_KISS (58) within 30yd while the player
-- carries the Warts-B-Gone Lip Balm (44986). player:HasItem,
-- creature:IsWithinDist (object.go generic fallback) and every aura leg are
-- bridged. Without the lip-balm aura (62574) the player gets SPELL_WARTS
-- (62581); with it, the frog casts SPELL_FROG_KISS (62536, non-triggered
-- DoCast twin) and 33211 takes SPELL_FROG_LOVE (62537). The
-- _following/_runningScript re-entry latch is a per-GUID table, and the
-- UpdateAI followup (:699-701 — the follower despawns once FROG_LOVE fades;
-- the aura lasts 60s) rides a 61s one-shot CreateLuaEvent, rechecked until
-- the aura is gone (C++ polls it every UpdateAI tick).
-- Ported 2026-10-24: the 33224 quest-frog branch — SetFacingToObject
-- via the atan2 + SetFacingTo twin + the EVENT_LAKEFROG_1..5
-- maiden-transform chain (DoCast 62550, UpdateEntry 33220, maiden
-- Talk SAY_MAIDEN_0/1, gossip-flag windows, despawn 4s after the last
-- Talk); the old "no bridges" line was stale — SetFacingTo,
-- UpdateEntry, SetFlag/RemoveFlag and Talk are all live on the
-- creature surface. Stays out: the kissed-frog MoveFollow leg
-- (zone_grizzly_hills.cpp:765) — MoveFollow IS bridged
-- (lua_creature_events.go:5610), so the leg is unported work, not
-- bridge-blocked (2026-11-13 correction) — and MoveIdle (vacuous —
-- the frog stands still).
-- PORTED 2026-11-13 (round 21): npc_venture_co_straggler
-- (zone_grizzly_hills.cpp:562-651) + npc_wounded_skirmisher
-- (:460-560), appended below. The 2026-10-24 "no entry constant,
-- DB ScriptName binding only, unverifiable" rationale is stale —
-- the DB-verification ruling governs (a creature_template row
-- carrying the exact C++ ScriptName IS the binding evidence), and
-- TDB_full_world_335.26091_2026_09_09.sql + the AC db_world dump
-- both bind 27463 'npc_wounded_skirmisher' and 27570
-- 'npc_venture_co_straggler'. The three MovePoint legs ride
-- creature:MoveTo (the MovePoint twin — the id is recorded so the
-- arrival fires MovementInform, ev28); DisappearAndDie rides
-- DespawnOrUnsummon (halazzi/marzon Despawn-twin precedent).
-- Documented-only (nearest-creature axis): npc_amberpine_woodsman
-- (zone_grizzly_hills.cpp:409-457): Reset's
-- FindNearestCreature(NPC_TALLHORN_STAG 26363, 0.2f) emote-state gate
-- (:420 — EMOTE_STATE_USE_STANDING vs. the EVENT_WOODSMAN_1/2
-- loot/attack emote loop) has a live bridge twin, but the AI is not
-- registered in boss_ai.go and carries no NPC_ entry constant anywhere
-- in the C++ tree. Entry is TDB-verifiable, not entry-blocked: TDB
-- binds 27293 'Amberpine Woodsman' to ScriptName
-- 'npc_amberpine_woodsman' (AC binds SmartAI on the same row) —
-- the "unverifiable — wounded-skirmisher precedent" citation is
-- stale (2026-11-13: the skirmisher itself was DB-verified and
-- ported round 21). Port pending as follow-up work; stays out here.
-- Ported (find-nearest-gameobject axis): npc_tallhorn_stag
-- (zone_grizzly_hills.cpp:339-390 — npc_tallhorn_stag : public
-- CreatureScript, registered in AddSC_zone_grizzly_hills()). Entry
-- NPC_TALLHORN_STAG = 26363 comes from the C++ AmberpineWoodsman enum
-- (zone_grizzly_hills.cpp:403-405), so the registration is C++-verifiable.
-- Ported: the Reset/UpdateAI _phase arm (:373-388) — the first UpdateAI tick
-- after Reset runs it exactly once (_phase 1 -> 0). It checks for the haunch
-- GO (OBJECT_HAUNCH = 188665, 2.0yd); when found the stag plays dead:
-- SetStandState(UNIT_STAND_STATE_DEAD = 7), SetImmuneToPC(true) (bridged as
-- creature:SetFlag(59, 0x100) — UNIT_FLAG_IMMUNE_TO_PC; the engine honors
-- 0x100 in the PC-targeting gate, creaturemotion.go:4213), and
-- SetUInt32Value(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_DEAD) (bridged as
-- creature:SetFlag(79, 0x20) — UNIT_DYNFLAG_DEAD is 0x20, kill.go:45; C++
-- sets the raw field while the twin ORs the bit, identical on the
-- zero-initialized spawn). The GO layout is static between Reset and the
-- first AI tick, so running the check synchronously on OnSpawn(5) /
-- OnReset(23) is C++-exact; the arm is idempotent, so both Reset call sites
-- are safe. The UpdateVictim/DoMeleeAttackIfReady tail is default engine
-- behavior — no port needed.
local ENTRY_LAKE_FROG = 33211
local ENTRY_LAKE_FROG_QUEST = 33224
local UNIT_NPC_FLAGS_FIELD = 82
local UNIT_NPC_FLAG_GOSSIP = 1
local SPELL_SUMMON_ASHWOOD_BRAND = 62554
local ENTRY_TALLHORN_STAG = 26363
local OBJECT_HAUNCH = 188665
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_IMMUNE_TO_PC = 0x100
local UNIT_DYNAMIC_FLAGS_FIELD = 79
local UNIT_DYNFLAG_DEAD = 0x20
local UNIT_STAND_STATE_DEAD = 7

local ITEM_WARTS_B_GONE_LIP_BALM = 44986
local SPELL_WARTSBGONE_LIP_BALM = 62574
local SPELL_FROG_LOVE = 62537
local SPELL_WARTS = 62581
local SPELL_FROG_KISS = 62536
local TEXT_EMOTE_KISS = 58
local ENTRY_MAIDEN_OF_ASHWOOD_LAKE = 33220
local SPELL_MAIDEN_TRANSFORM = 62550
local SAY_MAIDEN_0 = 0
local SAY_MAIDEN_1 = 1

local timers = {}
local frogKissLatched = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
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

local function lakeFrogSpawn(_, creature)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
end

local function lakeFrogGossipSelect(_, player, creature)
    creature:CastSpell(player, SPELL_SUMMON_ASHWOOD_BRAND)
    return false
end

-- C++ UpdateAI :699-701: the kissed follower despawns once SPELL_FROG_LOVE
-- fades. Rechecked until the aura is gone (C++ polls every UpdateAI tick).
local function frogLoveWatch(creature, guid)
    if creature:HasAura(SPELL_FROG_LOVE) then
        schedule(guid, "fadinglove", 61000, function()
            frogLoveWatch(creature, guid)
        end)
    else
        timers[guid] = nil
        frogKissLatched[guid] = nil
        creature:Despawn()
    end
end

-- C++ EVENT_LAKEFROG_1..5 chain (zone_grizzly_hills.cpp:721-743),
-- defined tail-first so no forward declaration is needed. Cadence:
-- 1 transform + entry change 2s after the kiss, 2 Talk 2s later,
-- 3 gossip flag 3s later, 4 flag removed 25s later, 5 Talk 2s later
-- + despawn 4s after that Talk.
local function lakeFrogChain5(creature, guid)
    creature:Talk(SAY_MAIDEN_1)
    schedule(guid, "lakefrogdespawn", 4000, function()
        timers[guid] = nil
        frogKissLatched[guid] = nil
        creature:Despawn()
    end)
end

local function lakeFrogChain4(creature, guid)
    creature:RemoveFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    schedule(guid, "lakefrog5", 2000, function()
        lakeFrogChain5(creature, guid)
    end)
end

local function lakeFrogChain3(creature, guid)
    creature:SetFlag(UNIT_NPC_FLAGS_FIELD, UNIT_NPC_FLAG_GOSSIP)
    schedule(guid, "lakefrog4", 25000, function()
        lakeFrogChain4(creature, guid)
    end)
end

local function lakeFrogChain2(creature, guid)
    creature:Talk(SAY_MAIDEN_0)
    schedule(guid, "lakefrog3", 3000, function()
        lakeFrogChain3(creature, guid)
    end)
end

local function lakeFrogChain1(creature, guid)
    creature:CastSpell(creature, SPELL_MAIDEN_TRANSFORM)
    creature:UpdateEntry(ENTRY_MAIDEN_OF_ASHWOOD_LAKE)
    schedule(guid, "lakefrog2", 2000, function()
        lakeFrogChain2(creature, guid)
    end)
end

local function lakeFrogReceiveEmote(_, creature, player, textEmote)
    local guid = creature:GetGUIDLow()
    if frogKissLatched[guid] then
        return
    end
    -- C++ IsWithinDistInMap(player, 30) compares raw 3D dist against
    -- 30 + own reach + target reach (Object.cpp _IsWithinDist); the
    -- emote source is a player (reach 1.5, ObjectDefines.h:40), so
    -- the twin is reach-exact; same-map gate vacuous here.
    if textEmote == TEXT_EMOTE_KISS and creature:GetDistance(player) - creature:GetCombatReach() - 1.5 < 30
        and player:HasItem(ITEM_WARTS_B_GONE_LIP_BALM, 1) then
        if not player:HasAura(SPELL_WARTSBGONE_LIP_BALM) then
            player:AddAura(SPELL_WARTS)
        else
            creature:CastSpell(player, SPELL_FROG_KISS)
            if creature:GetEntry() == ENTRY_LAKE_FROG then
                creature:AddAura(SPELL_FROG_LOVE)
                schedule(guid, "fadinglove", 61000, function()
                    frogLoveWatch(creature, guid)
                end)
            elseif creature:GetEntry() == ENTRY_LAKE_FROG_QUEST then
                -- C++ :766-775 quest-frog branch: MoveIdle is vacuous
                -- (no movement model — the frog stands still) +
                -- SetFacingToObject(player) via the atan2 + SetFacingTo
                -- twin (zuljin/devourer precedent), then the
                -- EVENT_LAKEFROG_1 chain 2s later (defined above).
                local dx = player:GetX() - creature:GetX()
                local dy = player:GetY() - creature:GetY()
                local face = math.atan2(dy, dx)
                if face < 0 then face = face + 2 * math.pi end
                creature:SetFacingTo(face)
                schedule(guid, "lakefrog1", 2000, function()
                    lakeFrogChain1(creature, guid)
                end)
            end
            frogKissLatched[guid] = true
        end
    end
end

-- Pure timer/latch hygiene (no C++ JustDied/Reset counterpart for these
-- legs): cancel the fading-love watch and clear the kiss latch.
local function lakeFrogDied(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    frogKissLatched[guid] = nil
end

local function lakeFrogReset(_, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    frogKissLatched[guid] = nil
end

RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 5, lakeFrogSpawn)
RegisterCreatureGossipEvent(ENTRY_LAKE_FROG, 2, lakeFrogGossipSelect)
RegisterCreatureGossipEvent(ENTRY_LAKE_FROG_QUEST, 2, lakeFrogGossipSelect)
-- The maiden is the same AI after UpdateEntry (C++ SetEntry keeps the
-- object), so its gossip select + died/reset handlers ride the same
-- functions; the died/reset pair also cancels a mid-chain kill's
-- pending timers instead of casting on a dead object.
RegisterCreatureGossipEvent(ENTRY_MAIDEN_OF_ASHWOOD_LAKE, 2, lakeFrogGossipSelect)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_ASHWOOD_LAKE, 4, lakeFrogDied)
RegisterCreatureEvent(ENTRY_MAIDEN_OF_ASHWOOD_LAKE, 23, lakeFrogReset)
RegisterCreatureEvent(ENTRY_LAKE_FROG, 8, lakeFrogReceiveEmote)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 8, lakeFrogReceiveEmote)
RegisterCreatureEvent(ENTRY_LAKE_FROG, 4, lakeFrogDied)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 4, lakeFrogDied)
RegisterCreatureEvent(ENTRY_LAKE_FROG, 23, lakeFrogReset)
RegisterCreatureEvent(ENTRY_LAKE_FROG_QUEST, 23, lakeFrogReset)

-- npc_tallhorn_stagAI::Reset -> Initialize (_phase = 1); the first
-- UpdateAI tick then evaluates the _phase arm once (:373-388).
local function tallhornStagReset(_, creature)
    if creature:FindNearestGameObject(OBJECT_HAUNCH, 2.0) ~= nil then
        creature:SetStandState(UNIT_STAND_STATE_DEAD)
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:SetFlag(UNIT_DYNAMIC_FLAGS_FIELD, UNIT_DYNFLAG_DEAD)
    end
end

RegisterCreatureEvent(ENTRY_TALLHORN_STAG, 5, tallhornStagReset)
RegisterCreatureEvent(ENTRY_TALLHORN_STAG, 23, tallhornStagReset)

-- spell_infected_worgen_bite (zone_grizzly_hills.cpp:839-868):
-- AuraScript on SPELL_INFECTED_WORGEN_BITE 53094. AfterEffectApply
-- EFFECT_1 SPELL_AURA_PERIODIC_DAMAGE, AURA_EFFECT_HANDLE_REAPPLY:
-- when the aura re-applies on a player at max stacks it removes
-- itself and the target casts SPELL_WORGENS_CALL 53095 on itself,
-- triggered. Go twin: the handler runs on ev2 (fresh apply) and ev4
-- (stack-raising merge) — together they cover every C++ reapply; a
-- refresh-only reapply at max stacks is unreachable (the aura is
-- removed the first time it reaches max). GetSpellStackAmount reads
-- DBC field 49 and is 0 until Spell.dbc ships 53094, so the arm is
-- dormant-but-correct until then (choking-vines convention).
-- No delta on the triggered flag: player:CastSpell funnels through
-- castSpellDirect, documented "Mirrors TrinityCore Unit::CastSpell
-- (Spell.cpp: triggered = true)" (spells.go:24323-24327); the `true`
-- arg is redundant, not dropped.
local SPELL_INFECTED_WORGEN_BITE = 53094
local SPELL_WORGENS_CALL = 53095

local function infectedWorgenBiteApply(_, aura, target)
    if target == nil or target:GetObjectType() ~= "Player" then
        return
    end
    local maxStacks = aura:GetSpellStackAmount(aura:GetAuraId())
    if maxStacks == 0 then
        return
    end
    if aura:GetStackAmount() ~= maxStacks then
        return
    end
    aura:Remove()
    target:CastSpell(target, SPELL_WORGENS_CALL, true)
end

RegisterAuraEvent(SPELL_INFECTED_WORGEN_BITE, 2, infectedWorgenBiteApply)
RegisterAuraEvent(SPELL_INFECTED_WORGEN_BITE, 4, infectedWorgenBiteApply)

-- spell_shredder_delivery (zone_grizzly_hills.cpp:801-836):
-- OnEffectHitTarget EFFECT_0 SPELL_EFFECT_SCRIPT_EFFECT; Load() gates
-- on the caster being a unit, and the handler despawns the caster when
-- it is a broken-down shredder (27354). ev5 is the twin
-- (boss_xevozz.lua:372 xevozzSummonPlayers precedent); the hit target
-- is unused — the registration is purely the trigger. DORMANT until
-- Spell.dbc ships 48610.
local SPELL_SHREDDER_DELIVERY = 48610 -- AC spell_script_names :1352
local NPC_BROKEN_DOWN_SHREDDER = 27354 -- zone_grizzly_hills.cpp:798

local function shredderDelivery(event, spell, hitTarget, effectIndex)
    if effectIndex ~= 0 then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil or caster:GetObjectType() ~= "Creature" then
        return
    end
    if caster:GetEntry() == NPC_BROKEN_DOWN_SHREDDER then
        caster:DespawnOrUnsummon()
    end
end

RegisterSpellEvent(SPELL_SHREDDER_DELIVERY, 5, shredderDelivery)

-- npc_wounded_skirmisher (zone_grizzly_hills.cpp:460-560) — SpellHit
-- (:503-524): player-cast SPELL_RENEW_SKIRMISHER 48812 with
-- QUEST_OVERWHELMED 12288 INCOMPLETE -> non-triggered DoCast(player,
-- SPELL_KILL_CREDIT 48813), Talk(SAY_RANDOM 0) to the player, then
-- standing -> MovePoint(1, +7,+7) with the MovementInform(1) despawn
-- 5s, else SetStandState(STAND) + DespawnOrUnsummon(5s). MovePoint
-- rides creature:MoveTo (the MovePoint twin — the id is recorded so
-- the arrival fires MovementInform, POINT_MOTION_TYPE 8, ev28).
-- Reset (:496-499) only re-inits the 5s timer constant — nothing to
-- port. UpdateAI (:527-533) is UpdateVictim + melee — engine
-- default, no port. Entry DB-verified: TDB + AC creature_template
-- rows bind 27463 to ScriptName 'npc_wounded_skirmisher'.
local ENTRY_WOUNDED_SKIRMISHER = 27463
local QUEST_OVERWHELMED = 12288
local SPELL_RENEW_SKIRMISHER = 48812
local SPELL_KILL_CREDIT = 48813
local QUEST_STATUS_INCOMPLETE = 3 -- quests.go:19 (sholazar precedent)
local SAY_RANDOM = 0
local UNIT_STAND_STATE_STAND = 0
local POINT_MOTION_TYPE = 8 -- MovementDefines.h:31 (flamecaller precedent)
local DESPAWN_TIMER_MS = 5000

local function woundedSkirmisherSpellHit(_, creature, caster, spellId)
    if spellId ~= SPELL_RENEW_SKIRMISHER or caster == nil then
        return
    end
    local player = caster:ToPlayer()
    if player == nil then
        return
    end
    if player:GetQuestStatus(QUEST_OVERWHELMED) ~= QUEST_STATUS_INCOMPLETE then
        return
    end
    creature:CastSpell(player, SPELL_KILL_CREDIT)
    creature:Talk(SAY_RANDOM, player)
    if creature:IsStandState() then
        creature:MoveTo(1, creature:GetX() + 7, creature:GetY() + 7, creature:GetZ())
    else
        creature:SetStandState(UNIT_STAND_STATE_STAND)
        creature:DespawnOrUnsummon(DESPAWN_TIMER_MS)
    end
end

-- C++ MovementInform (zone_grizzly_hills.cpp:501-505): id 1 ->
-- DespawnOrUnsummon(5s). The first arg is the motion type, gated on
-- POINT_MOTION_TYPE like the flamecaller ev28 handler.
local function woundedSkirmisherMovementInform(_, creature, moveType, pointId)
    if moveType ~= POINT_MOTION_TYPE or pointId ~= 1 then
        return
    end
    creature:DespawnOrUnsummon(DESPAWN_TIMER_MS)
end

RegisterCreatureEvent(ENTRY_WOUNDED_SKIRMISHER, 14, woundedSkirmisherSpellHit)
RegisterCreatureEvent(ENTRY_WOUNDED_SKIRMISHER, 28, woundedSkirmisherMovementInform)

-- npc_venture_co_straggler (zone_grizzly_hills.cpp:562-651).
-- SpellHit (:627-640): player-cast SPELL_SMOKE_BOMB 49075 ->
-- SetFlag NON_ATTACKABLE + SetImmuneToPC(true) (= SetFlag(59,
-- 0x100), tallhorn precedent) + SetReactState(REACT_PASSIVE) +
-- CombatStop(false) + store the caster GUID + EVENT_STRAGGLER_1 in
-- 3500ms. The chain (:587-625, tail-first below): 1 credit cast
-- 49093 on the stored player + MovePoint(0, -7,+7) + 2 in 2500ms;
-- 2 Talk(SAY_SEO 0) + MovePoint(0, -7,-5) + 3 in 2500ms; 3
-- MovePoint(0, -5,-5) + 4 in 2500ms; 4 DisappearAndDie, ported as
-- DespawnOrUnsummon(0) (halazzi/marzon Despawn-twin precedent).
-- MovePoint rides creature:MoveTo (id 0; the straggler has no
-- MovementInform override, so no ev28 handler is needed). Reset
-- (:578-586) clears the player GUID + RemoveFlag NON_ATTACKABLE +
-- SetImmuneToPC(false) + SetReactState(REACT_AGGRESSIVE).
-- JustEngagedWith (:574-577) schedules EVENT_CHOP in 3-6s; CHOP
-- (:619-623) casts SPELL_CHOP 43410 on the victim if any and repeats
-- in 10-12s. The chain + CHOP timers cancel on ev2/ev4/ev23 (the
-- file's cancelTimers idiom); C++ keeps the chain across evade, but
-- the despawn at the end of the chain makes the timers one-shot per
-- spawn in practice. Entry DB-verified: TDB + AC creature_template
-- rows bind 27570 to ScriptName 'npc_venture_co_straggler'.
local ENTRY_VENTURE_CO_STRAGGLER = 27570
local SPELL_SMOKE_BOMB = 49075
local SPELL_CHOP = 43410
local SPELL_VENTURE_STRAGGLER_CREDIT = 49093
local SAY_SEO = 0
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local stragglerPlayer = {}

local function stragglerStage4(creature, guid)
    creature:DespawnOrUnsummon(0)
    cancelTimers(guid)
    stragglerPlayer[guid] = nil
end

local function stragglerStage3(creature, guid)
    creature:MoveTo(0, creature:GetX() - 5, creature:GetY() - 5, creature:GetZ())
    schedule(guid, "straggler", 2500, function()
        stragglerStage4(creature, guid)
    end)
end

local function stragglerStage2(creature, guid)
    creature:Talk(SAY_SEO)
    creature:MoveTo(0, creature:GetX() - 7, creature:GetY() - 5, creature:GetZ())
    schedule(guid, "straggler", 2500, function()
        stragglerStage3(creature, guid)
    end)
end

local function stragglerStage1(creature, guid)
    local playerGuid = stragglerPlayer[guid]
    if playerGuid ~= nil then
        local player = GetPlayerByGUID(playerGuid)
        if player ~= nil then
            creature:CastSpell(player, SPELL_VENTURE_STRAGGLER_CREDIT)
        end
    end
    creature:MoveTo(0, creature:GetX() - 7, creature:GetY() + 7, creature:GetZ())
    schedule(guid, "straggler", 2500, function()
        stragglerStage2(creature, guid)
    end)
end

local function stragglerChop(creature, guid)
    local victim = creature:GetVictim()
    if victim ~= nil then
        creature:CastSpell(victim, SPELL_CHOP)
    end
    schedule(guid, "chop", {10000, 12000}, function()
        stragglerChop(creature, guid)
    end)
end

local function stragglerSpellHit(_, creature, caster, spellId)
    if spellId ~= SPELL_SMOKE_BOMB or caster == nil then
        return
    end
    local player = caster:ToPlayer()
    if player == nil then
        return
    end
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
    creature:SetReactState(REACT_PASSIVE)
    creature:CombatStop(false)
    local guid = creature:GetGUIDLow()
    stragglerPlayer[guid] = player:GetGUID()
    schedule(guid, "straggler", 3500, function()
        stragglerStage1(creature, guid)
    end)
end

local function stragglerEnterCombat(_, creature)
    local guid = creature:GetGUIDLow()
    schedule(guid, "chop", {3000, 6000}, function()
        stragglerChop(creature, guid)
    end)
end

local function stragglerCleanup(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    stragglerPlayer[guid] = nil
    if event == 23 then
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_PC)
        creature:SetReactState(REACT_AGGRESSIVE)
    end
end

RegisterCreatureEvent(ENTRY_VENTURE_CO_STRAGGLER, 1, stragglerEnterCombat)
RegisterCreatureEvent(ENTRY_VENTURE_CO_STRAGGLER, 2, stragglerCleanup)
RegisterCreatureEvent(ENTRY_VENTURE_CO_STRAGGLER, 4, stragglerCleanup)
RegisterCreatureEvent(ENTRY_VENTURE_CO_STRAGGLER, 14, stragglerSpellHit)
RegisterCreatureEvent(ENTRY_VENTURE_CO_STRAGGLER, 23, stragglerCleanup)
