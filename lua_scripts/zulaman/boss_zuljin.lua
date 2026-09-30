-- Zul'jin (Zul'Aman) — Lua port of
-- src/server/scripts/EasternKingdoms/ZulAman/boss_zuljin.cpp
-- (boss_zuljinAI + npc_zuljin_vortexAI); zulaman.h:33 (BOSS_ZULJIN = 5),
-- zulaman.h:51 (NPC_ZULJIN = 23863).
-- Creature entries: 23863 Zul'jin (C++ ScriptName "boss_zuljin" per
-- AddSC_boss_zuljin); 24136 feather vortex (C++ ScriptName
-- "npc_zuljin_vortex"). The four animal spirits (23878, 23880, 23877,
-- 23879) and the column of fire (24187) have no C++ CreatureScript in
-- this file and are not registered.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 14 OnSpellHit, 23 OnReset. Timers via
-- CreateLuaEvent; melee is engine-driven in Go (creature combat tick),
-- like C++ DoMeleeAttackIfReady.
-- Fight shape (C++ Phase is the 0-based form index: 0 troll, 1 bear,
-- 2 eagle, 3 lynx, 4 dragonhawk; transitions at 80/60/40/20% health via
-- health_20 = 20% of max):
--   troll: Talk YELL_INTRO, then YELL_AGGRO at 37s; whirlwind 17207 at
--     7s (repeat {15s,20s}); grievous throw 43093 at 8s on a random
--     player within 100 yd (repeat 10s).
--   bear: Talk TRANSFORM_TO_BEAR, triggered-adjacent self-cast 42594;
--     creeping paralysis 43095 at 7s after entering (repeat 20s).
--   eagle: Talk TRANSFORM_TO_EAGLE, self-cast 42606, triggered energy
--     storm 43983 aura; the boss stops moving (AttackStartNoMove) and the
--     four feather vortices fight (see below).
--   lynx: Talk TRANSFORM_TO_LYNX, self-cast 42607, energy storm removed;
--     claw rage starter at 5s after entering, lynx rush starter at 14s
--     (each repeat {15s,20s} after its sequence ends).
--   dragonhawk: Talk TRANSFORM_TO_DRAGONHAWK, self-cast 42608; flame
--     whirl 43213 at 5s (repeat 12s); pillar of fire 43216 at 7s on a
--     random player (repeat 10s); flame breath 43215 at 6s on self after
--     facing a random player (repeat 10s).
--   berserk 45078 one-shot at 600s with Talk YELL_BERSERK, then every
--     60s (C++-exact re-arm quirk).
-- Claw rage / lynx rush sequences (C++ TankGUID set while active): the
-- boss rushes between targets — claw rage lands 12 triggered 43150 hits
-- on a 500ms loop, lynx rush 9 triggered 43153 hits — then returns to
-- the tank. While a sequence is active the phase-transition check is
-- paused (C++ !TankGUID gate) and DoMeleeAttackIfReady is skipped.
-- Deviations from C++: no summon model — the four animal spirits, the
-- four feather vortices, and the column-of-fire pillars never spawn, so
-- SpawnAdds, the spirit Siphon Soul 43501 casts, the spirit death-state
-- arms (incl. JustDied's), and the vortex summon/cleanup arms are
-- skipped (the vortex AI is still registered so any spawned 24136
-- zaps back correctly); no teleport bridge — DoTeleportTo(CENTER) on
-- phase change skipped; no threat model — ResetThreatList skipped; no
-- equipment bridge — the UNIT_VIRTUAL_ITEM_SLOT_ID arms skipped; no
-- movement model — AttackStartNoMove (eagle), AttackStart victim
-- switches (claw/lynx), SetSpeedRate 5.0/1.2, and MoveChase (lynx
-- entry) have no bearer, so the rush sequences are modeled as
-- damage-only 500ms loops: claw rage hits the current victim (random
-- player fallback), lynx rush hits a random alive player per jump
-- (C++-exact per-hit spell, count, and 500ms cadence); the C++
-- interleaving when a sequence starter fires while the other sequence
-- is active (shared claw counter pumped by both loop bodies) is not
-- modeled — the starter is ignored until the active sequence ends;
-- the C++ melee-range gate on each sequence hit is approximated with
-- GetDistance <= 5 (no IsWithinMeleeRange bridge); the
-- isTargetableForAttack fallback chain collapses to victim-or-random
-- (no targetable bridge); the no-target EnterEvadeMode arm has no
-- bridge — the sequence ends and the starter re-arms instead; the bear
-- overpower arm has no bridge — it keys off a dodged melee swing
-- (AttackerStateUpdate health-unchanged check) with no per-swing Lua
-- hook, so the 5s overpower cooldown is not modeled; the eagle energy-
-- storm zap (42577 trigger on enemies in range) has no bearer — the
-- aura self-cast stays but nothing zaps; the vortex UpdateAI
-- melee-range re-target has no bridge (no AttackStart) — only its
-- SpellHit zap-back is modeled; no instance-script model — admission
-- only via the luaBossAI shim (GetZulAmanAI/BossAI SetBossState arms
-- skipped); YELL_FIRE_BREATH (6) is defined in the C++ enum but never
-- used by the AI — not modeled.

local ENTRY_ZULJIN = 23863
local ENTRY_VORTEX = 24136

local YELL_INTRO = 0
local YELL_AGGRO = 1
local YELL_BERSERK = 7
local YELL_KILL = 8
local YELL_DEATH = 9

local SPELL_WHIRLWIND = 17207
local SPELL_GRIEVOUS_THROW = 43093
local SPELL_CREEPING_PARALYSIS = 43095
local SPELL_ENERGY_STORM = 43983
local SPELL_ZAP_INFORM = 42577
local SPELL_ZAP_DAMAGE = 43137
local SPELL_CLAW_RAGE_DAMAGE = 43150
local SPELL_LYNX_RUSH_DAMAGE = 43153
local SPELL_FLAME_WHIRL = 43213
local SPELL_FLAME_BREATH = 43215
local SPELL_SUMMON_PILLAR = 43216
local SPELL_BERSERK = 45078

-- Indexed by the OLD (0-based) phase, like C++ Transform[Phase]: the
-- transform aura cast when leaving that phase for the next form.
local TRANSFORMS = {
    [0] = { text = 2, spell = 42594, unaura = SPELL_WHIRLWIND },
    [1] = { text = 3, spell = 42606, unaura = 42594 },
    [2] = { text = 4, spell = 42607, unaura = 42606 },
    [3] = { text = 5, spell = 42608, unaura = 42607 },
}

local PHASE_TROLL = 0
local PHASE_BEAR = 1
local PHASE_EAGLE = 2
local PHASE_LYNX = 3
local PHASE_DRAGONHAWK = 4

local MELEE_RANGE = 5

local timers = {}
local state = {}

local function cancelTimers(store, guid)
    local per = store[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        store[guid] = nil
    end
end

local function schedule(store, guid, key, delay, fn)
    local per = store[guid]
    if not per then
        per = {}
        store[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function zuljinState(guid)
    local st = state[guid]
    if not st then
        st = { phase = PHASE_TROLL, introActive = false, sequence = nil,
               seqCount = 0 }
        state[guid] = st
    end
    return st
end

local function alivePlayers(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomPlayer(creature, maxDist)
    local candidates = {}
    for _, p in ipairs(alivePlayers(creature)) do
        if maxDist == nil or creature:GetDistance(p) <= maxDist then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ berserk: one-shot 600s, Talk, triggered self-cast 45078; the C++
-- timer then re-arms for 60s (not 600s).
local function onBerserk(creature, guid)
    creature:Talk(YELL_BERSERK)
    creature:CastSpell(creature, SPELL_BERSERK, true)
    schedule(timers, guid, "berserk", 60000, function()
        onBerserk(creature, guid)
    end)
end

-- C++ intro timer: 37s after combat, Talk YELL_AGGRO (only runs in troll
-- form; leaving phase 0 first kills the pending yell C++-exact, and the
-- KilledUnit gate below stays shut).
local function onIntro(creature, guid)
    creature:Talk(YELL_AGGRO)
    zuljinState(guid).introActive = false
end

-- C++ whirlwind (troll): DoCast(me, 17207), repeat {15s,20s}.
local function onWhirlwind(creature, guid)
    creature:CastSpell(creature, SPELL_WHIRLWIND)
    schedule(timers, guid, "whirlwind", math.random(15000, 20000), function()
        onWhirlwind(creature, guid)
    end)
end

-- C++ grievous throw (troll): random player within 100 yd, DoCast
-- 43093; the 10s re-arm happens even with no target.
local function onGrievousThrow(creature, guid)
    local target = randomPlayer(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_GRIEVOUS_THROW)
    end
    schedule(timers, guid, "grievous", 10000, function()
        onGrievousThrow(creature, guid)
    end)
end

-- C++ creeping paralysis (bear): DoCast(me, 43095), repeat 20s.
local function onCreepingParalysis(creature, guid)
    creature:CastSpell(creature, SPELL_CREEPING_PARALYSIS)
    schedule(timers, guid, "paralysis", 20000, function()
        onCreepingParalysis(creature, guid)
    end)
end

-- Shared claw-rage / lynx-rush 500ms loop (C++ Claw_Loop_Timer).
-- claw: hits the current victim (random alive player fallback) with
-- triggered 43150, 12 hits; lynx: a random alive player per jump with
-- triggered 43153, 9 hits. No-target ends the sequence (the C++
-- EnterEvadeMode arm has no bridge).
local onSeqLoop
local onClawRage
local onLynxRush
onSeqLoop = function(creature, guid)
    local st = zuljinState(guid)
    local seq = st.sequence
    if seq == nil then
        return
    end
    local target
    if seq == "claw" then
        target = creature:GetVictim() or randomPlayer(creature)
    else
        target = randomPlayer(creature)
    end
    if target == nil or creature:GetDistance(target) > MELEE_RANGE then
        if target == nil then
            st.sequence = nil
            schedule(timers, guid, seq == "claw" and "clawrage" or "lynxrush",
                math.random(15000, 20000), function()
                    if seq == "claw" then
                        onClawRage(creature, guid)
                    else
                        onLynxRush(creature, guid)
                    end
                end)
            return
        end
        schedule(timers, guid, "seqloop", 500, function()
            onSeqLoop(creature, guid)
        end)
        return
    end
    if seq == "claw" then
        creature:CastSpell(target, SPELL_CLAW_RAGE_DAMAGE, true)
    else
        creature:CastSpell(target, SPELL_LYNX_RUSH_DAMAGE, true)
    end
    st.seqCount = st.seqCount + 1
    local done = (seq == "claw" and st.seqCount >= 12)
        or (seq == "lynx" and st.seqCount >= 9)
    if done then
        st.sequence = nil
        local key = seq == "claw" and "clawrage" or "lynxrush"
        local starter = seq == "claw" and onClawRage or onLynxRush
        schedule(timers, guid, key, math.random(15000, 20000), function()
            starter(creature, guid)
        end)
        return
    end
    schedule(timers, guid, "seqloop", 500, function()
        onSeqLoop(creature, guid)
    end)
end

-- C++ claw rage starter (lynx): needs a target; starts the 500ms loop.
-- Ignored while a sequence is already active (the C++ shared-counter
-- interleaving is not modeled).
onClawRage = function(creature, guid)
    local st = zuljinState(guid)
    if st.sequence ~= nil then
        return
    end
    if #alivePlayers(creature) == 0 then
        schedule(timers, guid, "clawrage", 1000, function()
            onClawRage(creature, guid)
        end)
        return
    end
    st.sequence = "claw"
    st.seqCount = 0
    schedule(timers, guid, "seqloop", 500, function()
        onSeqLoop(creature, guid)
    end)
end

-- C++ lynx rush starter (lynx): same shape as claw rage, 9-hit loop.
onLynxRush = function(creature, guid)
    local st = zuljinState(guid)
    if st.sequence ~= nil then
        return
    end
    if #alivePlayers(creature) == 0 then
        schedule(timers, guid, "lynxrush", 1000, function()
            onLynxRush(creature, guid)
        end)
        return
    end
    st.sequence = "lynx"
    st.seqCount = 0
    schedule(timers, guid, "seqloop", 500, function()
        onSeqLoop(creature, guid)
    end)
end

-- C++ flame whirl (dragonhawk): DoCast(me, 43213), repeat 12s.
local function onFlameWhirl(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_WHIRL)
    schedule(timers, guid, "flamework", 12000, function()
        onFlameWhirl(creature, guid)
    end)
end

-- C++ pillar of fire (dragonhawk): random player, DoCast 43216;
-- re-arms even with no target.
local function onPillar(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_SUMMON_PILLAR)
    end
    schedule(timers, guid, "pillar", 10000, function()
        onPillar(creature, guid)
    end)
end

-- C++ flame breath (dragonhawk): face a random player (no bridge),
-- DoCast(me, 43215); re-arms even with no target.
local function onFlameBreath(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_BREATH)
    schedule(timers, guid, "flamebreath", 10000, function()
        onFlameBreath(creature, guid)
    end)
end

-- Phase timer keys canceled on every transition (berserk persists).
local PHASE_TIMER_KEYS = { "intro", "whirlwind", "grievous", "paralysis",
    "clawrage", "lynxrush", "seqloop", "flamework", "flamebreath", "pillar" }

-- C++ EnterPhase(NextPhase): teleport/threat/equipment/AttackStart arms
-- have no bridge; the transform unaura removal, transform cast, yell,
-- energy-storm cast/removal, and per-phase timer arming are modeled.
-- Phase timers start ticking only on phase entry (C++ decrements them
-- only in their phase's UpdateAI case).
local function enterPhase(creature, guid, next)
    local st = zuljinState(guid)
    local old = st.phase
    for _, key in ipairs(PHASE_TIMER_KEYS) do
        local per = timers[guid]
        if per and per[key] then
            RemoveEventById(per[key])
            per[key] = nil
        end
    end
    st.phase = next
    st.sequence = nil
    st.seqCount = 0
    local t = TRANSFORMS[old]
    if t then
        creature:RemoveAura(t.unaura)
        creature:CastSpell(creature, t.spell)
        creature:Talk(t.text)
    end
    if next == PHASE_EAGLE then
        creature:CastSpell(creature, SPELL_ENERGY_STORM, true)
    elseif next == PHASE_LYNX then
        creature:RemoveAura(SPELL_ENERGY_STORM)
        schedule(timers, guid, "clawrage", 5000, function()
            onClawRage(creature, guid)
        end)
        schedule(timers, guid, "lynxrush", 14000, function()
            onLynxRush(creature, guid)
        end)
    elseif next == PHASE_BEAR then
        schedule(timers, guid, "paralysis", 7000, function()
            onCreepingParalysis(creature, guid)
        end)
    elseif next == PHASE_DRAGONHAWK then
        schedule(timers, guid, "flamework", 5000, function()
            onFlameWhirl(creature, guid)
        end)
        schedule(timers, guid, "flamebreath", 6000, function()
            onFlameBreath(creature, guid)
        end)
        schedule(timers, guid, "pillar", 7000, function()
            onPillar(creature, guid)
        end)
    end
end

local function zuljinResetState(guid)
    cancelTimers(timers, guid)
    state[guid] = { phase = PHASE_TROLL, introActive = false,
                    sequence = nil, seqCount = 0 }
end

-- C++ JustEngagedWith: Talk YELL_INTRO, SpawnAdds (no summon model),
-- EnterPhase(0) (no-op).
local function zuljinEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    zuljinResetState(guid)
    local st = zuljinState(guid)
    st.introActive = true
    creature:Talk(YELL_INTRO)
    schedule(timers, guid, "intro", 37000, function()
        onIntro(creature, guid)
    end)
    schedule(timers, guid, "berserk", 600000, function()
        onBerserk(creature, guid)
    end)
    schedule(timers, guid, "whirlwind", 7000, function()
        onWhirlwind(creature, guid)
    end)
    schedule(timers, guid, "grievous", 8000, function()
        onGrievousThrow(creature, guid)
    end)
end

local function zuljinLeaveCombat(event, creature)
    zuljinResetState(creature:GetGUID())
end

-- C++ KilledUnit: suppressed while the intro timer is pending; no
-- player gate in C++.
local function zuljinTargetDied(event, creature, victim)
    if zuljinState(creature:GetGUID()).introActive then
        return
    end
    creature:Talk(YELL_KILL)
end

local function zuljinDied(event, creature, killer)
    zuljinResetState(creature:GetGUID())
    creature:Talk(YELL_DEATH)
end

local function zuljinReset(event, creature)
    zuljinResetState(creature:GetGUID())
end

-- C++ UpdateAI phase check: when no rush sequence is active and
-- pre-damage health < health_20 * (4 - Phase), EnterPhase(Phase + 1).
-- health_20 = floor(maxHealth * 20 / 100) (C++ CountPctFromMaxHealth).
local function zuljinDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = zuljinState(guid)
    if st.sequence ~= nil or st.phase >= PHASE_DRAGONHAWK then
        return false, damage
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return false, damage
    end
    local health20 = math.floor(maxHealth * 20 / 100)
    if health < health20 * (4 - st.phase) then
        enterPhase(creature, guid, st.phase + 1)
    end
    return false, damage
end

RegisterCreatureEvent(ENTRY_ZULJIN, 1, zuljinEnterCombat)
RegisterCreatureEvent(ENTRY_ZULJIN, 2, zuljinLeaveCombat)
RegisterCreatureEvent(ENTRY_ZULJIN, 3, zuljinTargetDied)
RegisterCreatureEvent(ENTRY_ZULJIN, 4, zuljinDied)
RegisterCreatureEvent(ENTRY_ZULJIN, 9, zuljinDamageTaken)
RegisterCreatureEvent(ENTRY_ZULJIN, 23, zuljinReset)

-- npc_zuljin_vortex (24136, C++ ScriptName "npc_zuljin_vortex"):
-- SpellHit on ZAP_INFORM 42577 -> triggered 43137 on the caster.
-- Vortices never spawn in this build (no summon model), but the AI is
-- registered so any that exist zap back correctly. The C++ UpdateAI
-- melee-range re-target has no bridge (no AttackStart) and is not
-- modeled.
local function vortexSpellHit(event, creature, caster, spellId)
    if spellId == SPELL_ZAP_INFORM and caster ~= nil then
        creature:CastSpell(caster, SPELL_ZAP_DAMAGE, true)
    end
end

RegisterCreatureEvent(ENTRY_VORTEX, 14, vortexSpellHit)
