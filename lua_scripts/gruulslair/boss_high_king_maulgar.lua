-- High King Maulgar (Gruul's Lair) — Lua port of
-- src/server/scripts/Outland/GruulsLair/boss_high_king_maulgar.cpp
-- (boss_high_king_maulgar, boss_olm_the_summoner, boss_kiggler_the_
-- crazed, boss_blindeye_the_seer, boss_krosh_firehand);
-- gruuls_lair.h (NPC_MAULGAR = 18831, NPC_KROSH_FIREHAND = 18832,
-- NPC_OLM_THE_SUMMONER = 18834, NPC_KIGGLER_THE_CRAZED = 18835,
-- NPC_BLINDEYE_THE_SEER = 18836). Entries: 18831 / 18832 / 18834 /
-- 18835 / 18836 — all entry+ScriptName verifiable from the C++
-- sources per AddSC_boss_high_king_maulgar; the creature_template
-- ScriptName bindings are DB-side (no TDB in this workspace).
-- Talk lines used: Maulgar SAY_AGGRO=0 (pull), SAY_ENRAGE=1
-- (phase 2), SAY_OGRE_DEATH=2 (ogre death — cross-creature arm,
-- documented only), SAY_SLAY=3 (kill), SAY_DEATH=4 (death). The
-- four ogre councillors have no Talk calls in C++.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers via CreateLuaEvent;
-- melee is engine-driven in Go (creature combat tick), like C++
-- DoMeleeAttackIfReady. C++ DoCast default is triggered=false
-- (vaelastrasz convention); DoCastVictim takes nil as the victim
-- arm.
-- Fight shape (C++-exact for the modeled arms): Maulgar OnReset
-- (23): per-GUID reset (ArcingSmash 10s / MightyBlow 40s /
-- Whirlwind 30s / Charging 0s / Roar 0s / Phase2=false),
-- non-triggered self-cast dual wield 29651 (C++ DoCast(me, SPELL_
-- DUAL_WIELD, false), C++-exact); the SetBossState(NOT_STARTED)
-- arm is blocked on the instance-script model. OnEnterCombat(1):
-- per-GUID reset + Talk(SAY_AGGRO). Arming at pull mirrors the
-- C++ UpdateAI timers. Maulgar phase 2 (a 1s recurring health
-- check, C++-exact transition rule, once-guarded): <50% health
-- -> Phase2=true, Talk(SAY_ENRAGE), triggered self-cast 29651
-- (C++-exact); the UNIT_VIRTUAL_ITEM_SLOT_ID zeroing arms have
-- no item-display bridge (blocked). Phase 2 charging: random
-- target, DoCast(target, SPELL_BERSERKER_C 26561), repeat 20s
-- (initial 0s) — the AttackStart(target) victim-switch arm has
-- no bridge, so the victim arm stays engine-side. Phase 2 roar:
-- non-triggered self-cast 16508, repeat {40s,50s} (initial 0s).
-- The DoAction(ACTION_ADD_DEATH) -> Talk(SAY_OGRE_DEATH) relay
-- has no cross-creature bridge — the four ogre JustDied DoAction
-- arms are unmodeled and SAY_OGRE_DEATH fires only if the engine
-- delivers a DoAction call (blocked).
-- Olm the Summoner (18834): OnReset(23) arms dark decay 33129 10s
-- then 20s (non-triggered DoCastVictim) / summon WFH 33131 15s
-- then 30s (non-triggered self-cast — the spell's summon is
-- engine-side; the Wild Fel Hunter has no AI class in this file,
-- so nothing further is registered, firesworn convention) /
-- death coil 33130 20s then 20s (non-triggered on a random
-- alive player in the instance — the C++ SelectTarget(Random, 0)
-- has no player-only gate and no threat bridge, teron/krosh
-- convention; nil pick casts nothing but keeps the schedule).
-- Olm's AttackStart override (0.0f threat + MoveChase 30 yd) has
-- no threat/movement bridges — the default engine AttackStart
-- applies (blocked). Kiggler the Crazed (18835): greater
-- polymorph 33173 5s then {15s,20s} (random-target convention) /
-- lightning bolt 36152 10s then 15s (non-triggered DoCastVictim)
-- / arcane shock 33175 20s then 20s / arcane explosion 33237 30s
-- then 30s — C++-exact DoCastVictim on all four. Blindeye the
-- Seer (18836): greater PW shield 33147 5s then 40s (self-cast) /
-- heal 33144 {25s,40s} then {15s,40s} (self-cast) / prayer of
-- healing 33152 {45s,55s} then {35s,50s} (self-cast).
-- Krosh Firehand (18832): greater fireball 33051 every 2s —
-- the C++ within-30-yd IsWithinDist gate has no distance bridge,
-- so the timer floor (2s) drives the cast (documented);
-- spell shield 33054 5s then 30s (C++-exact DoCastVictim; the
-- InterruptNonMeleeSpells arm has no interrupt bridge, blocked);
-- blast wave 33061 20s then 60s — the C++ threat-list-within-15-
-- yd pick has no threat/distance bridges, so a random alive
-- player in the instance is used (teron convention), non-
-- triggered on the pick, nil pick casts nothing (burn
-- convention); the InterruptNonMeleeSpells arm is blocked.
-- All five: OnEnterCombat(1) resets per-GUID state and re-arms
-- the combat timers (the DoZoneInCombat arms have no zone-combat
-- bridge; the SetBossState(IN_PROGRESS) arms are blocked on the
-- instance-script model). OnDied(4): Maulgar Talk(SAY_DEATH);
-- the ogre JustDied cross-creature DoAction relays and the
-- SetBossState(DONE) arms are blocked — timers are cancelled
-- and per-GUID state dropped. OnTargetDied(3): Maulgar Talk(
-- SAY_SLAY), no TYPEID gate (C++-exact). OnLeaveCombat(2)/
-- OnReset(23): cancel timers, drop per-GUID state (evade-side
-- cleanup is engine-side). Melee is engine-driven for all five.

local NPC_MAULGAR = 18831
local NPC_KROSH_FIREHAND = 18832
local NPC_OLM_THE_SUMMONER = 18834
local NPC_KIGGLER_THE_CRAZED = 18835
local NPC_BLINDEYE_THE_SEER = 18836

local SPELL_ARCING_SMASH = 39144
local SPELL_MIGHTY_BLOW = 33230
local SPELL_WHIRLWIND = 33238
local SPELL_BERSERKER_C = 26561
local SPELL_ROAR = 16508
local SPELL_DUAL_WIELD = 29651

local SPELL_DARK_DECAY = 33129
local SPELL_DEATH_COIL = 33130
local SPELL_SUMMON_WFH = 33131

local SPELL_GREATER_POLYMORPH = 33173
local SPELL_LIGHTNING_BOLT = 36152
local SPELL_ARCANE_SHOCK = 33175
local SPELL_ARCANE_EXPLOSION = 33237

local SPELL_GREATER_PW_SHIELD = 33147
local SPELL_HEAL = 33144
local SPELL_PRAYER_OH = 33152

local SPELL_GREATER_FIREBALL = 33051
local SPELL_SPELLSHIELD = 33054
local SPELL_BLAST_WAVE = 33061

local SAY_AGGRO = 0
local SAY_ENRAGE = 1
local SAY_SLAY = 3
local SAY_DEATH = 4

local timers = {}
local maulgarState = {}

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

-- Alive players sharing the creature's map+instance (teron
-- convention).
local function playersInInstance(creature)
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

local function randomPlayer(creature)
    local candidates = playersInInstance(creature)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(1, #candidates)]
end

-- Maulgar arms: arcing smash / whirlwind / mighty blow are
-- non-triggered DoCastVictim on their C++ repeats.

local function maulgarArcingSmash(creature, guid)
    creature:CastSpell(nil, SPELL_ARCING_SMASH)
    schedule(guid, "arcingsmash", 10000, function()
        maulgarArcingSmash(creature, guid)
    end)
end

local function maulgarWhirlwind(creature, guid)
    creature:CastSpell(nil, SPELL_WHIRLWIND)
    schedule(guid, "whirlwind", 55000, function()
        maulgarWhirlwind(creature, guid)
    end)
end

local function maulgarMightyBlow(creature, guid)
    creature:CastSpell(nil, SPELL_MIGHTY_BLOW)
    schedule(guid, "mightyblow", math.random(30000, 40000), function()
        maulgarMightyBlow(creature, guid)
    end)
end

-- C++ phase 2 charging arm: random target, DoCast(target, SPELL_
-- BERSERKER_C); repeat 20s (initial 0s). The AttackStart victim-
-- switch arm has no bridge.
local function maulgarCharging(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_BERSERKER_C)
    end
    schedule(guid, "charging", 20000, function()
        maulgarCharging(creature, guid)
    end)
end

-- C++ phase 2 roar: non-triggered self-cast 16508; repeat
-- {40s,50s} (initial 0s).
local function maulgarRoar(creature, guid)
    creature:CastSpell(creature, SPELL_ROAR)
    schedule(guid, "roar", math.random(40000, 50000), function()
        maulgarRoar(creature, guid)
    end)
end

-- C++ UpdateAI phase-2 gate, once-guarded: <50% health ->
-- Talk(SAY_ENRAGE) + triggered self-cast 29651 + the charging/
-- roar arms (the virtual-item-slot zeroing arms have no
-- item-display bridge).
local function maulgarPhaseCheck(creature, guid)
    local state = maulgarState[guid]
    if state == nil or state.phase2 then
        return
    end
    if creature:GetHealthPct() < 50 then
        state.phase2 = true
        creature:Talk(SAY_ENRAGE)
        creature:CastSpell(creature, SPELL_DUAL_WIELD, true)
        schedule(guid, "charging", 0, function()
            maulgarCharging(creature, guid)
        end)
        schedule(guid, "roar", 0, function()
            maulgarRoar(creature, guid)
        end)
        return
    end
    schedule(guid, "phasecheck", 1000, function()
        maulgarPhaseCheck(creature, guid)
    end)
end

local function maulgarResetState(guid)
    cancelTimers(guid)
    maulgarState[guid] = nil
end

local function maulgarArmCombat(creature, guid)
    schedule(guid, "arcingsmash", 10000, function()
        maulgarArcingSmash(creature, guid)
    end)
    schedule(guid, "whirlwind", 30000, function()
        maulgarWhirlwind(creature, guid)
    end)
    schedule(guid, "mightyblow", 40000, function()
        maulgarMightyBlow(creature, guid)
    end)
    schedule(guid, "phasecheck", 1000, function()
        maulgarPhaseCheck(creature, guid)
    end)
end

-- C++ JustEngagedWith: Talk(SAY_AGGRO) + the three combat arms
-- and the phase-2 poller; the DoZoneInCombat and
-- SetBossState(IN_PROGRESS) arms are blocked.
local function maulgarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    maulgarResetState(guid)
    maulgarState[guid] = { phase2 = false }
    creature:Talk(SAY_AGGRO)
    maulgarArmCombat(creature, guid)
end

local function maulgarLeaveCombat(event, creature)
    maulgarResetState(creature:GetGUID())
end

-- C++ KilledUnit: Talk(SAY_SLAY), no victim gate (C++-exact).
local function maulgarTargetDied(event, creature, victim)
    creature:Talk(SAY_SLAY)
end

-- C++ JustDied: Talk(SAY_DEATH); the SetBossState(DONE) arm is
-- blocked on the instance-script model.
local function maulgarDied(event, creature, killer)
    creature:Talk(SAY_DEATH)
    maulgarResetState(creature:GetGUID())
end

-- C++ Reset: Initialize + non-triggered self-cast dual wield
-- 29651 (C++-exact — unlike Brutallus this one is NOT
-- triggered); the SetBossState(NOT_STARTED) arm is blocked.
local function maulgarReset(event, creature)
    local guid = creature:GetGUID()
    maulgarResetState(guid)
    maulgarState[guid] = { phase2 = false }
    creature:CastSpell(creature, SPELL_DUAL_WIELD)
end

RegisterCreatureEvent(NPC_MAULGAR, 1, maulgarEnterCombat)
RegisterCreatureEvent(NPC_MAULGAR, 2, maulgarLeaveCombat)
RegisterCreatureEvent(NPC_MAULGAR, 3, maulgarTargetDied)
RegisterCreatureEvent(NPC_MAULGAR, 4, maulgarDied)
RegisterCreatureEvent(NPC_MAULGAR, 23, maulgarReset)

-- Olm the Summoner arms.

local function olmDarkDecay(creature, guid)
    creature:CastSpell(nil, SPELL_DARK_DECAY)
    schedule(guid, "darkdecay", 20000, function()
        olmDarkDecay(creature, guid)
    end)
end

-- C++ summon arm: non-triggered DoCast(me, SPELL_SUMMON_WFH).
-- The WFH spawn is a spell effect (engine-side); the Wild Fel
-- Hunter has no AI class in this file, so nothing else is
-- registered (firesworn convention).
local function olmSummon(creature, guid)
    creature:CastSpell(creature, SPELL_SUMMON_WFH)
    schedule(guid, "summon", 30000, function()
        olmSummon(creature, guid)
    end)
end

local function olmDeathCoil(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_DEATH_COIL)
    end
    schedule(guid, "deathcoil", 20000, function()
        olmDeathCoil(creature, guid)
    end)
end

local function olmResetState(guid)
    cancelTimers(guid)
end

local function olmArmCombat(creature, guid)
    schedule(guid, "darkdecay", 10000, function()
        olmDarkDecay(creature, guid)
    end)
    schedule(guid, "summon", 15000, function()
        olmSummon(creature, guid)
    end)
    schedule(guid, "deathcoil", 20000, function()
        olmDeathCoil(creature, guid)
    end)
end

-- C++ JustEngagedWith: DoZoneInCombat (no zone-combat bridge)
-- + SetBossState(IN_PROGRESS) (blocked) + the three combat
-- arms.
local function olmEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    olmResetState(guid)
    olmArmCombat(creature, guid)
end

local function olmLeaveCombat(event, creature)
    olmResetState(creature:GetGUID())
end

-- C++ JustDied: the maulgar DoAction(ACTION_ADD_DEATH) relay
-- and the SetBossState(DONE) arm are blocked on the
-- cross-creature and instance-script models — timer cleanup
-- only.
local function olmDied(event, creature, killer)
    olmResetState(creature:GetGUID())
end

-- C++ Reset: Initialize the three timers; the
-- SetBossState(NOT_STARTED) arm is blocked. The AttackStart
-- override (0.0f threat + MoveChase 30 yd) has no threat/
-- movement bridges — default engine AttackStart applies.
local function olmReset(event, creature)
    olmResetState(creature:GetGUID())
end

RegisterCreatureEvent(NPC_OLM_THE_SUMMONER, 1, olmEnterCombat)
RegisterCreatureEvent(NPC_OLM_THE_SUMMONER, 2, olmLeaveCombat)
RegisterCreatureEvent(NPC_OLM_THE_SUMMONER, 4, olmDied)
RegisterCreatureEvent(NPC_OLM_THE_SUMMONER, 23, olmReset)

-- Kiggler the Crazed arms.

local function kigglerPolymorph(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_GREATER_POLYMORPH)
    end
    schedule(guid, "polymorph", math.random(15000, 20000), function()
        kigglerPolymorph(creature, guid)
    end)
end

local function kigglerLightningBolt(creature, guid)
    creature:CastSpell(nil, SPELL_LIGHTNING_BOLT)
    schedule(guid, "lightningbolt", 15000, function()
        kigglerLightningBolt(creature, guid)
    end)
end

local function kigglerArcaneShock(creature, guid)
    creature:CastSpell(nil, SPELL_ARCANE_SHOCK)
    schedule(guid, "arcaneshock", 20000, function()
        kigglerArcaneShock(creature, guid)
    end)
end

-- C++-exact: DoCastVictim even for the AoE (arcane explosion).
local function kigglerArcaneExplosion(creature, guid)
    creature:CastSpell(nil, SPELL_ARCANE_EXPLOSION)
    schedule(guid, "arcaneexplosion", 30000, function()
        kigglerArcaneExplosion(creature, guid)
    end)
end

local function kigglerResetState(guid)
    cancelTimers(guid)
end

local function kigglerEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    kigglerResetState(guid)
    schedule(guid, "polymorph", 5000, function()
        kigglerPolymorph(creature, guid)
    end)
    schedule(guid, "lightningbolt", 10000, function()
        kigglerLightningBolt(creature, guid)
    end)
    schedule(guid, "arcaneshock", 20000, function()
        kigglerArcaneShock(creature, guid)
    end)
    schedule(guid, "arcaneexplosion", 30000, function()
        kigglerArcaneExplosion(creature, guid)
    end)
end

local function kigglerLeaveCombat(event, creature)
    kigglerResetState(creature:GetGUID())
end

-- C++ JustDied: the maulgar DoAction relay and
-- SetBossState(DONE) are blocked — timer cleanup only.
local function kigglerDied(event, creature, killer)
    kigglerResetState(creature:GetGUID())
end

local function kigglerReset(event, creature)
    kigglerResetState(creature:GetGUID())
end

RegisterCreatureEvent(NPC_KIGGLER_THE_CRAZED, 1, kigglerEnterCombat)
RegisterCreatureEvent(NPC_KIGGLER_THE_CRAZED, 2, kigglerLeaveCombat)
RegisterCreatureEvent(NPC_KIGGLER_THE_CRAZED, 4, kigglerDied)
RegisterCreatureEvent(NPC_KIGGLER_THE_CRAZED, 23, kigglerReset)

-- Blindeye the Seer arms (all self-casts).

local function blindeyeShield(creature, guid)
    creature:CastSpell(creature, SPELL_GREATER_PW_SHIELD)
    schedule(guid, "shield", 40000, function()
        blindeyeShield(creature, guid)
    end)
end

local function blindeyeHeal(creature, guid)
    creature:CastSpell(creature, SPELL_HEAL)
    schedule(guid, "heal", math.random(15000, 40000), function()
        blindeyeHeal(creature, guid)
    end)
end

local function blindeyePrayer(creature, guid)
    creature:CastSpell(creature, SPELL_PRAYER_OH)
    schedule(guid, "prayer", math.random(35000, 50000), function()
        blindeyePrayer(creature, guid)
    end)
end

local function blindeyeResetState(guid)
    cancelTimers(guid)
end

local function blindeyeEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    blindeyeResetState(guid)
    schedule(guid, "shield", 5000, function()
        blindeyeShield(creature, guid)
    end)
    schedule(guid, "heal", math.random(25000, 40000), function()
        blindeyeHeal(creature, guid)
    end)
    schedule(guid, "prayer", math.random(45000, 55000), function()
        blindeyePrayer(creature, guid)
    end)
end

local function blindeyeLeaveCombat(event, creature)
    blindeyeResetState(creature:GetGUID())
end

-- C++ JustDied: the maulgar DoAction relay and
-- SetBossState(DONE) are blocked — timer cleanup only.
local function blindeyeDied(event, creature, killer)
    blindeyeResetState(creature:GetGUID())
end

local function blindeyeReset(event, creature)
    blindeyeResetState(creature:GetGUID())
end

RegisterCreatureEvent(NPC_BLINDEYE_THE_SEER, 1, blindeyeEnterCombat)
RegisterCreatureEvent(NPC_BLINDEYE_THE_SEER, 2, blindeyeLeaveCombat)
RegisterCreatureEvent(NPC_BLINDEYE_THE_SEER, 4, blindeyeDied)
RegisterCreatureEvent(NPC_BLINDEYE_THE_SEER, 23, blindeyeReset)

-- Krosh Firehand arms.

-- C++ greater fireball: fires when the timer lapses OR the
-- victim is within 30 yd; the IsWithinDist gate has no distance
-- bridge, so the 2s timer floor drives the cast (documented).
local function kroshFireball(creature, guid)
    creature:CastSpell(nil, SPELL_GREATER_FIREBALL)
    schedule(guid, "fireball", 2000, function()
        kroshFireball(creature, guid)
    end)
end

-- C++-exact: DoCastVictim(33054) (the InterruptNonMeleeSpells
-- arm has no interrupt bridge, blocked).
local function kroshSpellShield(creature, guid)
    creature:CastSpell(nil, SPELL_SPELLSHIELD)
    schedule(guid, "spellshield", 30000, function()
        kroshSpellShield(creature, guid)
    end)
end

-- C++ blast wave: random threat-list target within 15 yd. The
-- threat-list and 15-yd picks have no bridges, so a random
-- alive player in the instance is used (teron convention); nil
-- pick casts nothing but keeps the schedule (burn convention).
-- The InterruptNonMeleeSpells arm has no interrupt bridge.
local function kroshBlastWave(creature, guid)
    local target = randomPlayer(creature)
    if target then
        creature:CastSpell(target, SPELL_BLAST_WAVE)
    end
    schedule(guid, "blastwave", 60000, function()
        kroshBlastWave(creature, guid)
    end)
end

local function kroshResetState(guid)
    cancelTimers(guid)
end

local function kroshEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    kroshResetState(guid)
    schedule(guid, "fireball", 1000, function()
        kroshFireball(creature, guid)
    end)
    schedule(guid, "spellshield", 5000, function()
        kroshSpellShield(creature, guid)
    end)
    schedule(guid, "blastwave", 20000, function()
        kroshBlastWave(creature, guid)
    end)
end

local function kroshLeaveCombat(event, creature)
    kroshResetState(creature:GetGUID())
end

-- C++ JustDied: the maulgar DoAction relay and
-- SetBossState(DONE) are blocked — timer cleanup only.
local function kroshDied(event, creature, killer)
    kroshResetState(creature:GetGUID())
end

local function kroshReset(event, creature)
    kroshResetState(creature:GetGUID())
end

RegisterCreatureEvent(NPC_KROSH_FIREHAND, 1, kroshEnterCombat)
RegisterCreatureEvent(NPC_KROSH_FIREHAND, 2, kroshLeaveCombat)
RegisterCreatureEvent(NPC_KROSH_FIREHAND, 4, kroshDied)
RegisterCreatureEvent(NPC_KROSH_FIREHAND, 23, kroshReset)
