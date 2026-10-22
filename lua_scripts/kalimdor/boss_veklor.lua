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
-- - Shadow Bolt 26006 (DoCastVictim): C++ ShadowBolt_Timer = 0
--   (boss_twinemperors.cpp:503) fires on the first UpdateAI tick —
--   first Lua cast at 1ms (CreateLuaEvent floor, runtime.go:1118),
--   then 2s repeat. C++ (lines 544-552): cast only when the
--   victim is within 45y (else MoveChase at VEKLOR_DIST 20 — no
--   motion bridge), with an unconditional 2s re-arm — modeled
--   as a distance-gated cast with unconditional re-arm.
-- - Blizzard 26607: C++ SelectTarget Random, 0, 45.0f, true ->
--   DoCast(target, SPELL_BLIZZARD) (boss_twinemperors.cpp:557),
--   init urand(15s,20s), re-arm 15000 + rand32()%15000. The 45y
--   eligibility gate is bridged (IsWithinDist — the old "no
--   distance bridge" claim was stale, retracted this run), so
--   the filtered random-player pick (vem/maiden_of_virtue
--   convention) is C++-exact.
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
-- - Arcane Burst 568: PORTED (the old "no range bridge" claim was
--   stale — GetDistance / IsWithinDist are bridged, blizzard-arm
--   precedent): nearest alive player within NOMINAL_MELEE_RANGE
--   (5.0f) via the same GetPlayersInWorld pool, non-triggered
--   DoCast on the pick, null-guarded like the C++ if; 1s init,
--   5s re-arm on cast, 500ms retry poll on empty (C++ leaves the
--   timer <= diff on no target — retried next UpdateAI tick, the
--   krosh distance-poller idiom). The unbridged TeleportToMyBrother
--   gate (TryActivateAfterTTelep / AfterTeleport 5s re-arm) stays
--   out, as does the sibling teleport arm.
-- - HandleBugs / CastSpellOnBug (SPELL_EXPLODEBUG 804): creature
--   grid enumeration + SetFaction + AddAura — no bridges.
-- - Shadow Bolt MoveChase leg (VEKLOR_DIST 20, "VL will not come
--   to melee when attacking"): motion bridge absent.
-- - Custom AttackStart (melee-less aggro): motion bridge absent.
-- - KilledUnit/JustEngagedWith/JustDied DoPlaySoundToSet
--   (SOUND_VL_AGGRO 8657 / KILL 8658 / DEATH 8659): ported via
--   creature:PlayDirectSound (the sound bridge is live —
--   DoPlaySoundToSet == PlayDirectSound, ScriptedCreature.cpp).
--   The !pOtherBoss->IsInCombat() aggro gate and the cross-AI
--   DontYellWhenDead death latch have no cross-AI bridge, so
--   aggro/death sounds play per-creature; zero Talk() in C++.
local ENTRY = 15276
local ENTRY_VEKNILASH = 15275 -- temple_of_ahnqiraj.h:75

-- UNIT_DYNAMIC_FLAGS (UpdateFields.h:136: OBJECT_END+0x49 = 79);
-- UNIT_DYNFLAG_LOOTABLE (SharedDefines.h:3128); the field-79 flag bridge
-- is live (lua_creature_events.go; opera + eredar-twins precedents) —
-- the old "no bridge" claim on the mirror-kill leg is retracted 2026-10-22.
local UNIT_DYNAMIC_FLAGS = 79
local UNIT_DYNFLAG_LOOTABLE = 0x0001

local SPELL_SHADOWBOLT = 26006
local SPELL_BLIZZARD = 26607
local SPELL_BERSERK = 26662

local SPELL_ARCANEBURST = 568

local SOUND_VL_AGGRO = 8657
local SOUND_VL_KILL = 8658
local SOUND_VL_DEATH = 8659

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

-- C++ ShadowBolt arm (boss_twinemperors.cpp:544-552): cast only
-- when the victim is within 45y (else MoveChase at VEKLOR_DIST
-- 20 — motion leg, no bridge); the 2s re-arm is unconditional
-- on both arms.
local function onShadowBolt(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:IsWithinDist(victim, 45) then
        creature:CastSpell(victim, SPELL_SHADOWBOLT)
    end
    schedule(guid, "sb", 2000, function()
        onShadowBolt(creature, guid)
    end)
end

-- C++ Blizzard arm (boss_twinemperors.cpp:554-560): SelectTarget
-- Random,0,45.0f,true -> DoCast(target, SPELL_BLIZZARD), init
-- urand(15000,20000), re-arm 15000 + rand32()%15000 ([15000,
-- 29999]). The 45y eligibility gate is bridged (IsWithinDist —
-- garaxxas precedent).
local function onBlizzard(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:IsWithinDist(p, 45) then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates > 0 then
        creature:CastSpell(candidates[math.random(#candidates)], SPELL_BLIZZARD)
    end
    schedule(guid, "blizzard", math.random(15000, 29999), function()
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

-- C++ Arcane Burst arm (boss_twinemperors.cpp:562-567):
-- SelectTarget MinDistance, 0, NOMINAL_MELEE_RANGE (5.0f),
-- playerOnly=true -> DoCast(target, SPELL_ARCANEBURST) 1-arg
-- (non-triggered); 1s init; re-arm 5s only on a successful cast
-- (on no target the timer is left <= diff, retried next UpdateAI
-- tick — emulated as a bounded 500ms retry poll, the krosh
-- distance-poller idiom).
local function onArcaneBurst(creature, guid)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local nearest, nearestDist = nil, nil
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            local d = creature:GetDistance(p)
            if d <= 5.0 and (nearestDist == nil or d < nearestDist) then
                nearest, nearestDist = p, d
            end
        end
    end
    local delay = 500
    if nearest then
        creature:CastSpell(nearest, SPELL_ARCANEBURST)
        delay = 5000
    end
    schedule(guid, "arcaneburst", delay, function()
        onArcaneBurst(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    flags[guid] = {}
    -- C++ JustEngagedWith: DoPlaySoundToSet(me, SOUND_VL_AGGRO 8657)
    -- behind the !pOtherBoss->IsInCombat() gate (cross-AI — no
    -- bridge); per-creature port.
    creature:PlayDirectSound(SOUND_VL_AGGRO)
    schedule(guid, "sb", 1, function()
        onShadowBolt(creature, guid)
    end)
    schedule(guid, "arcaneburst", 1000, function()
        onArcaneBurst(creature, guid)
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
    -- C++ boss_veklorAI::Reset() :520: ApplySpellImmune(0,
    -- IMMUNITY_DAMAGE, SPELL_SCHOOL_MASK_NORMAL=1, true) — Vek'lor is
    -- immune to physical damage (Vek'nilash takes the magic side).
    creature:ApplySpellImmune(0, 3, 1, true)
end

-- C++ KilledUnit: DoPlaySoundToSet(me, SOUND_VL_KILL 8658), ungated.
local function onKilledUnit(event, creature, victim)
    creature:PlayDirectSound(SOUND_VL_KILL)
end

-- C++ JustDied: DoPlaySoundToSet(me, SOUND_VL_DEATH 8659) behind the
-- cross-AI DontYellWhenDead latch (no cross-AI bridge — plays
-- per-creature); _JustDied() is the instance-model leg.
local function onDied(event, creature, killer)
    creature:PlayDirectSound(SOUND_VL_DEATH)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_twinemperorsAI::DamageTaken (boss_twinemperors.cpp:115-128):
-- the struck twin mirrors a health-proportional share of the hit onto
-- the other twin (dPercent from own max health, odmg from the twin's max
-- health, (int) truncation); fires on every hit incl. the killing blow,
-- so no postDamage guard. The setDeathState(JUST_DIED) leg has no
-- bridge (engine-side death handling) and stays out; the
-- UNIT_DYNFLAG_LOOTABLE leg on the mirror-killed twin PORTS via
-- other:SetFlag(79, 0x0001) (field 79 honored, lua_creature_events.go;
-- opera + eredar-twins precedents) — old "no bridge" claim retracted
-- 2026-10-22.
local function onDamageTaken(event, creature, attacker, damage)
    local other = creature:FindNearestCreature(ENTRY_VEKNILASH, 150.0)
    if other == nil then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local odmg = math.floor(damage / maxHealth * other:GetMaxHealth())
    local ohealth = other:GetHealth() - odmg
    local mirrorKilled = ohealth <= 0 -- unclamped, C++ :122
    if ohealth < 0 then
        ohealth = 0
    end
    other:SetHealth(ohealth)
    -- C++ boss_twinemperors.cpp:124-128: the mirror-killed twin gets
    -- UNIT_DYNFLAG_LOOTABLE (the setDeathState leg has no bridge).
    if mirrorKilled then
        other:SetFlag(UNIT_DYNAMIC_FLAGS, UNIT_DYNFLAG_LOOTABLE)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onKilledUnit)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
