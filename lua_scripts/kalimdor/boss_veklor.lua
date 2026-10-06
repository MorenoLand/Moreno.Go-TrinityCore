-- Temple of Ahn'Qiraj: Emperor Vek'lor — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_twinemperors.cpp
-- (boss_veklorAI : public boss_twinemperorsAI; GetAI via
-- GetAQ40AI<boss_veklorAI>(creature); registered in
-- AddSC_boss_twinemperors(); kalimdor loader decl 94 — eighth
-- "// Temple of ahn'qiraj" group, after skeram, before ouro).
-- Entry: 15276 = NPC_VEKLOR (temple_of_ahnqiraj.h:74; the
-- creature_template ScriptName binding stays DB-side).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 23 OnReset. Timers via CreateLuaEvent. C++ does NOT melee
-- (DoMeleeAttackIfReady is commented out; custom AttackStart
-- does MoveChase at VEKLOR_DIST 20 — motion leg, no bridge).
-- Ported arms (the self-contained in-combat legs):
-- - Shadow Bolt 26006 (DoCastVictim): 2s repeat. The <45y
--   MoveChase leg has no motion bridge; ported as fixed 2s
--   victim-cast (fankriss convention).
-- - Blizzard 26607: C++ SelectTarget Random, 0, 45.0f, true ->
--   DoCast(target, SPELL_BLIZZARD), init urand(15s,20s) ->
--   urand(15s,30s). Random-player pick via GetPlayersInWorld
--   (huhuran convention — no SelectTarget bridge).
-- - Berserk 26662 (CheckEnrage): DoCast self, init 15min ->
--   repeat 60min. The IsNonMeleeSpellCast gate has no bridge —
--   fixed timer.
-- Unmodeled (documented-only, no bridges):
-- - TwinReset / BossAI ctor leg DATA_TWIN_EMPERORS: instance
--   bridge, standing.
-- - Shared health (DamageTaken %mirroring via GetOtherBoss +
--   SpellHit SPELL_HEAL_BROTHER 7393 rebalance): cross-boss via
--   instance->GetCreature(DATA_VEKNILASH) — no instance bridge.
--   VN never casts TryHealBrother anyway (IAmVeklor early-out),
--   but the twin heal/rebalance machinery is cross-AI.
-- - TeleportToMyBrother + SetAfterTeleport + TryActivateAfterTTelep:
--   position swap + stun/interrupt/ResetThreatList + twin visual
--   cast 26638/800 + post-teleport ArcaneBurst 5s re-arm —
--   no position/SelectTarget/InterruptNonMeleeSpells/
--   ResetThreatList/AttackStart bridges.
-- - Arcane Burst 568: C++ DoCast on nearest MELEE-range target
--   (SelectTarget MinDistance, 0, NOMINAL_MELEE_RANGE, true),
--   1s init -> 5s repeat — no range bridge; melee-proximity
--   detection is the whole point of the spell, so unlike
--   Uppercut (kri victim-cast convention) this stays unmodeled.
-- - HandleBugs / CastSpellOnBug (SPELL_EXPLODEBUG 804): creature
--   grid enumeration + SetFaction + AddAura — no bridges.
-- - Shadow Bolt MoveChase leg (VEKLOR_DIST 20, "VL will not come
--   to melee when attacking"): motion bridge absent.
-- - Custom AttackStart (melee-less aggro): motion bridge absent.
-- - KilledUnit/JustEngagedWith/JustDied DoPlaySoundToSet
--   (SOUND_VL_AGGRO 8657 / KILL 8658 / DEATH 8659): no sound
--   bridge; zero Talk() in C++.
local ENTRY = 15276

local SPELL_SHADOWBOLT = 26006
local SPELL_BLIZZARD = 26607
local SPELL_BERSERK = 26662

local timers = {}
local flags = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    flags[guid] = nil
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

-- C++ ShadowBolt leg: DoCastVictim(SPELL_SHADOWBOLT) when victim
-- within 45y (else MoveChase), 2s repeat. Motion leg: no bridge;
-- ported as fixed 2s victim-cast (fankriss convention).
local function onShadowBolt(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHADOWBOLT)
    end
    schedule(guid, "sb", 2000, function()
        onShadowBolt(creature, guid)
    end)
end

-- C++ EVENT_BLIZZARD: SelectTarget Random,0,45.0f,true ->
-- DoCast(target, SPELL_BLIZZARD), init urand(15000,20000) ->
-- urand(15000,30000). Random-player pick via GetPlayersInWorld
-- (huhuran convention).
local function onBlizzard(creature, guid)
    local players = GetPlayersInWorld()
    if #players > 0 then
        local target = players[math.random(#players)]
        creature:CastSpell(target, SPELL_BLIZZARD)
    end
    schedule(guid, "blizzard", math.random(15000, 30000), function()
        onBlizzard(creature, guid)
    end)
end

-- C++ CheckEnrage: DoCast(me, SPELL_BERSERK) at 15min, re-armed
-- to 60min. The IsNonMeleeSpellCast gate has no bridge — fixed
-- timer.
local function onBerserk(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK)
    schedule(guid, "berserk", 3600000, function()
        onBerserk(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    flags[guid] = {}
    schedule(guid, "sb", 2000, function()
        onShadowBolt(creature, guid)
    end)
    schedule(guid, "blizzard", math.random(15000, 20000), function()
        onBlizzard(creature, guid)
    end)
    schedule(guid, "berserk", 900000, function()
        onBerserk(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    cancelTimers(creature:GetGUID())
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 23, onReset)
