-- Temple of Ahn'Qiraj: Emperor Vek'nilash — Lua port of
-- src/server/scripts/Kalimdor/TempleOfAhnQiraj/boss_twinemperors.cpp
-- (boss_veknilashAI : public boss_twinemperorsAI; GetAI via
-- GetAQ40AI<boss_veknilashAI>(creature); registered in
-- AddSC_boss_twinemperors(); kalimdor loader decl 94 — eighth
-- "// Temple of ahn'qiraj" group, after skeram, before ouro).
-- Entry: 15275 = NPC_VEKNILASH (temple_of_ahnqiraj.h:75; the
-- creature_template ScriptName binding stays DB-side).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 9 OnDamageTaken, 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Unbalancing Strike 26613 (DoCastVictim): init urand(8s,18s)
--   -> urand(8s,20s). Victim cast (jeklik convention).
-- - Uppercut 26007: C++ DoCast on a random MELEE-range target
--   (SelectTarget Random, 0, NOMINAL_MELEE_RANGE 5.0f, true), init
--   urand(14s,29s) -> urand(15s,30s). The stale "no range/
--   SelectTarget bridge" claim is retracted: the pick is a random
--   alive player within 5y via the bridged IsWithinDist check
--   (the skadi MaxDistance precedent for distance-filtered picks).
-- - Berserk 26662 (CheckEnrage): DoCast self, init 15min ->
--   repeat 60min. The IsNonMeleeSpellCast gate has no bridge —
--   fixed timer.
-- Unmodeled (documented-only, no bridges):
-- - TwinReset / BossAI ctor leg DATA_TWIN_EMPERORS: instance
--   bridge, standing.
-- - Shared health, DamageTaken half PORTED (boss_veklor precedent):
--   the C++ GetOtherBoss leg (instance->GetCreature(DATA_VEKLOR))
--   is reached via the bridged creature:FindNearestCreature
--   (150y, entry-unique per map — the veklor-port idiom); the
--   ohealth<=0 setDeathState(JUST_DIED) + DYNFLAG_LOOTABLE legs stay
--   out (engine-side death handling, no bridge). The SpellHit
--   SPELL_HEAL_BROTHER 7393 rebalance half stays out (cross-AI
--   GetOtherBoss + SetHealth rebalance math — no bridge).
-- - TryHealBrother: VN casts 7393 on VEKLOR when within 60y —
--   cross-boss, no bridge.
-- - TeleportToMyBrother + SetAfterTeleport + TryActivateAfterTTelep:
--   position swap + stun/interrupt/ResetThreatList + twin visual
--   cast 26638 + post-teleport threat arm — no position/SelectTarget/
--   InterruptNonMeleeSpells/ResetThreatList/AttackStart bridges.
-- - HandleBugs / CastSpellOnBug (SPELL_MUTATE_BUG 802): creature
--   grid enumeration + SetFaction + threat-victim AttackStart +
--   AddAura — no bridges.
-- - KilledUnit/JustEngagedWith/JustDied DoPlaySoundToSet
--   (SOUND_VN_AGGRO 8661 / KILL 8662 / DEATH 8660): ported via
--   creature:PlayDirectSound (the sound bridge is live —
--   DoPlaySoundToSet == PlayDirectSound, ScriptedCreature.cpp).
--   The !pOtherBoss->IsInCombat() aggro gate and the cross-AI
--   DontYellWhenDead death latch have no cross-AI bridge, so
--   aggro/death sounds play per-creature; zero Talk() in C++.
local ENTRY = 15275
local ENTRY_VEKLOR = 15276 -- temple_of_ahnqiraj.h:74

local SPELL_UNBALANCING_STRIKE = 26613
local SPELL_UPPERCUT = 26007
local SPELL_BERSERK = 26662

local SOUND_VN_DEATH = 8660
local SOUND_VN_AGGRO = 8661
local SOUND_VN_KILL = 8662

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

-- C++ EVENT_UNBALANCING_STRIKE: DoCastVictim(SPELL_UNBALANCING_STRIKE),
-- init urand(8000,18000) -> 8000 + rand32()%12000 = [8000,19999].
local function onUnbalancingStrike(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_UNBALANCING_STRIKE)
    end
    schedule(guid, "us", math.random(8000, 19999), function()
        onUnbalancingStrike(creature, guid)
    end)
end

-- C++ EVENT_UPPERCUT: DoCast on random melee-range target, init
-- urand(14000,29000) -> 15000 + rand32()%15000 = [15000,29999].
-- Random alive player within 5y (NOMINAL_MELEE_RANGE).
local function randomMeleePlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() and creature:IsWithinDist(plr, 5.0) then
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onUppercut(creature, guid)
    local target = randomMeleePlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_UPPERCUT)
    end
    schedule(guid, "uppercut", math.random(15000, 29999), function()
        onUppercut(creature, guid)
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
    -- C++ JustEngagedWith: DoPlaySoundToSet(me, SOUND_VN_AGGRO 8661)
    -- behind the !pOtherBoss->IsInCombat() gate (cross-AI — no
    -- bridge); per-creature port.
    creature:PlayDirectSound(SOUND_VN_AGGRO)
    schedule(guid, "us", math.random(8000, 18000), function()
        onUnbalancingStrike(creature, guid)
    end)
    schedule(guid, "uppercut", math.random(14000, 29000), function()
        onUppercut(creature, guid)
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
    -- C++ boss_veknilashAI::Reset() :429: ApplySpellImmune(0,
    -- IMMUNITY_DAMAGE, SPELL_SCHOOL_MASK_MAGIC=126, true) — Vek'nilash
    -- is immune to magic damage (Vek'lor takes the physical side).
    creature:ApplySpellImmune(0, 3, 126, true)
end

-- C++ KilledUnit: DoPlaySoundToSet(me, SOUND_VN_KILL 8662), ungated.
local function onKilledUnit(event, creature, victim)
    creature:PlayDirectSound(SOUND_VN_KILL)
end

-- C++ JustDied: DoPlaySoundToSet(me, SOUND_VN_DEATH 8660) behind the
-- cross-AI DontYellWhenDead latch (no cross-AI bridge — plays
-- per-creature); _JustDied() is the instance-model leg.
local function onDied(event, creature, killer)
    creature:PlayDirectSound(SOUND_VN_DEATH)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_twinemperorsAI::DamageTaken (boss_twinemperors.cpp:115-128):
-- the struck twin mirrors a health-proportional share of the hit onto
-- the other twin (dPercent from own max health, odmg from the twin's max
-- health, (int) truncation); fires on every hit incl. the killing blow,
-- so no postDamage guard. The setDeathState(JUST_DIED) +
-- UNIT_DYNFLAG_LOOTABLE legs on the 0-hp twin have no bridge
-- (engine-side death handling) and stay out. Mirrors the veklor port.
local function onDamageTaken(event, creature, attacker, damage)
    local other = creature:FindNearestCreature(ENTRY_VEKLOR, 150.0)
    if other == nil then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local odmg = math.floor(damage / maxHealth * other:GetMaxHealth())
    local ohealth = other:GetHealth() - odmg
    if ohealth < 0 then
        ohealth = 0
    end
    other:SetHealth(ohealth)
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onKilledUnit)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY, 23, onReset)
