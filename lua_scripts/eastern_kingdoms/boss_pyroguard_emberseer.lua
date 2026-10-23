-- Pyroguard Emberseer (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_pyroguard_emberseer.cpp
-- (boss_pyroguard_emberseer — BossAI combat scheduler; the file's
-- second class, npc_blackhand_incarcerator, is ported separately in
-- npc_blackhand_incarcerator.lua). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (AddSC_boss_pyroguardemberseer
-- follows AddSC_boss_quatermasterzigris in the Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_PYROGAURD_EMBERSEER = 9816 in the BRS creatures enum (BRSDataTypes
-- DATA_PYROGAURD_EMBERSEER = 9). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace), but the entry itself
-- is C++-verifiable so the port is registered (quartermaster_zigris /
-- boss_the_beast precedent). GetBlackrockSpireAI is a GetInstanceAI
-- retrieval wrapper only (blackrock_spire.h:129-132).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI combat
-- event machine):
-- EVENT_FIRE_SHIELD: self-cast FIRE_SHIELD 13376, 3s init -> 3s loop
-- (DoCast(me) path — creature:CastSpell(creature, spell),
-- twilight_corrupter convention).
-- EVENT_FIRENOVA: self-cast FIRENOVA 23462, 6s init -> 6s loop.
-- EVENT_FLAMEBUFFET: self-cast FLAMEBUFFET 23341, 3s init -> 14s loop.
-- EVENT_PYROBLAST: random-player PYROBLAST 17274 — SelectTarget(Random,
-- 0, 100, true) is playerOnly, ported via the keleseth GetPlayersInWorld
-- precedent (same map/instance, alive, within 100yd); 14s init -> 15s
-- loop, re-arm unconditional (C++ Repeat sits outside the target
-- check), non-triggered DoCast(target).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (creature
-- casts are packet-visual), so timers fire unconditionally and both
-- in-loop casting gates are dropped (maiden precedent). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention). C++ schedules EVENT_FIRE_SHIELD at Reset and
-- ticks it out of combat as well; the port starts it on
-- OnEnterCombat(1) — the out-of-combat ticks are unmodeled.
-- Unmodeled (documented-only): the whole pre-fight event is gated on
-- instance / SetData bridges that do not exist on the Lua surface
-- (SpellHit IS bridged — event 14 — but the SpellHit arms are dead
-- code in the ported model: 15281 is self-cast by the unported
-- incarcerator machine, and 16048's trigger is unmodeled) —
-- RemoveAura(16047/16048/16049) + EVENT_RESPAWN 5s (instance->SetData
-- (DATA_BLACKHAND_INCARCERATOR, 1) + SetBossState(NOT_STARTED));
-- (Reset's UNIT_FLAG_NOT_SELECTABLE + SetImmuneToPC(true) pair IS
-- ported — creature:SetFlag(59, 0x2000000) / SetFlag(59, 0x100),
-- boss_pyroguard_emberseer.cpp:85-86.)
-- SetData(type, 1) -> EVENT_PLAYER_CHECK 5s (player-list aura scan
-- for SPELL_EMBERSEER_OBJECT_VISUAL 16532, instance->SetBossState
-- (IN_PROGRESS)); EVENT_PRE_FIGHT_1 (GetCreatureListWithEntryInGrid
-- NPC_BLACKHAND_INCARCERATOR 10316, 35.0f -> SetImmuneToAll(false) +
-- InterruptSpell + DoZoneInCombat, RemoveAura(15282),
-- EVENT_PRE_FIGHT_2 at 32s); EVENT_PRE_FIGHT_2 (self-cast 16245 +
-- 16048, Talk(EMOTE_ONE_STACK)); EVENT_ENTER_COMBAT (AttackStart(
-- SelectNearestPlayer(30.0f))); SpellHit 15281 (SPELL_ENCAGE_EMBERSEER) -> self-cast 15282 (SPELL_ENCAGED_EMBERSEER) +
-- Reset if no aura; SpellHit 16049 stack latches (EMOTE_TEN_STACK at
-- 10, at 20: remove 16245, cast 16047, Talk(EMOTE_FREE_OF_BONDS /
-- YELL_FREE_OF_BONDS), clear flags, EVENT_ENTER_COMBAT at 2s)
-- (instance / SetData / SelectTarget precedents: instance / the_beast /
-- draenei_survivor).
-- Reset's _Reset() and JustDied's _JustDied() are internal
-- BossAI machinery covered by the cancel/re-arm on combat events
-- (halycon precedent); JustDied's UpdateRunes(GO_STATE_READY) +
-- instance->SetBossState(DONE) arms are instance-side — unmodeled
-- (instance precedent).

local SPELL_FIRE_SHIELD = 13376
local SPELL_FIRENOVA = 23462
local SPELL_FLAMEBUFFET = 23341
local SPELL_PYROBLAST = 17274

local ENTRY_PYROGAURD_EMBERSEER = 9816

local timers = {}

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

local function onFireShield(creature, guid)
    creature:CastSpell(creature, SPELL_FIRE_SHIELD)
    schedule(guid, "fireshield", 3000, function() onFireShield(creature, guid) end)
end

local function onFireNova(creature, guid)
    creature:CastSpell(creature, SPELL_FIRENOVA)
    schedule(guid, "firenova", 6000, function() onFireNova(creature, guid) end)
end

local function onFlameBuffet(creature, guid)
    creature:CastSpell(creature, SPELL_FLAMEBUFFET)
    schedule(guid, "flamebuffet", 14000, function() onFlameBuffet(creature, guid) end)
end

-- C++ SelectTarget(Random, 0, dist, true) playerOnly=true arms ride the
-- keleseth GetPlayersInWorld precedent.
local function randomPlayerInRange(creature, range)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and creature:IsWithinDist(plr, range) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_PYROBLAST: SelectTarget(Random, 0, 100, true) playerOnly,
-- DoCast(target, 17274) non-triggered, 14s init, Repeat(15s) outside
-- the target check (re-arm unconditional).
local function onPyroblast(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_PYROBLAST)
    end
    schedule(guid, "pyroblast", 15000, function() onPyroblast(creature, guid) end)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fireshield", 3000, function() onFireShield(creature, guid) end)
    schedule(guid, "firenova", 6000, function() onFireNova(creature, guid) end)
    schedule(guid, "flamebuffet", 3000, function() onFlameBuffet(creature, guid) end)
    schedule(guid, "pyroblast", 14000, function() onPyroblast(creature, guid) end)
end

local function onCombatEnd(_, creature)
    -- C++ Reset (boss_pyroguard_emberseer.cpp:83-86): NOT_SELECTABLE +
    -- SetImmuneToPC(true) [SetFlag(59, 0x100)] — the flag pair comes
    -- first in arm order, before events.Reset and the aura cleanup.
    creature:SetFlag(59, 0x2000000)
    creature:SetFlag(59, 0x100)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY_PYROGAURD_EMBERSEER, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_PYROGAURD_EMBERSEER, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_PYROGAURD_EMBERSEER, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_PYROGAURD_EMBERSEER, 23, onCombatEnd)
