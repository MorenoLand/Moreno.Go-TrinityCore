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
-- (phase 2), SAY_OGRE_DEATH=2 (ogre death — ported: each ogre
-- JustDied relays via relayOgreDeath below), SAY_SLAY=3 (kill),
-- SAY_DEATH=4 (death). The
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
-- (C++-exact); the UNIT_VIRTUAL_ITEM_SLOT_ID zeroing arms ride
-- creature:SetEquipmentSlots (slot 2 skipped, EQUIP_NO_CHANGE).
-- Phase 2 charging: random target, DoCast(target, SPELL_BERSERKER_C
-- 26561), repeat 20s
-- (initial 0s) — the AttackStart(target) victim-switch arm has
-- no bridge, so the victim arm stays engine-side. Phase 2 roar:
-- non-triggered self-cast 16508, repeat {40s,50s} (initial 0s).
-- The DoAction(ACTION_ADD_DEATH) -> Talk(SAY_OGRE_DEATH) relay
-- ports via relayOgreDeath below: no DoAction bridge exists, but
-- the receiver lookup is the bridged FindNearestCreature (svalna
-- precedent) and the receiver leg is a bare Talk, so the relay is
-- C++-exact for a live maulgar; only the instance SetBossState
-- (DONE) leg stays blocked.
-- Olm the Summoner (18834): OnReset(23) arms dark decay 33129 10s
-- then 20s (non-triggered DoCastVictim) / summon WFH 33131 15s
-- then 30s (non-triggered self-cast — the spell's summon is
-- engine-side; the Wild Fel Hunter has no AI class in this file,
-- so nothing further is registered, firesworn convention) /
-- death coil 33130 20s then 20s (non-triggered on a random threat-list
-- target — the C++ SelectTarget(Random, 0) has no player-only gate
-- (UnitAI.h:170); threat-list pick via GetAITargets, no Player filter;
-- nil pick casts nothing but keeps the schedule).
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
-- Krosh Firehand (18832): greater fireball 33051 — the 2s
-- floor drives the timer arm, a 500ms distance poller the C++
-- within-30-yd (3D) early-fire OR (boss_high_king_maulgar.cpp:
-- 538-543; both arms re-arm 2000 — the old "no distance bridge"
-- claim was stale, IsWithinDist is bridged, retracted this run);
-- spell shield 33054 5s then 30s (C++-exact DoCastVictim; the
-- InterruptNonMeleeSpells(false) arm ported below;
-- blast wave 33061 20s then 60s — the C++ threat-list-within-15-
-- yd (2D) pick via GetAITargets (threat-list bridge),
-- non-triggered on the pick, the pick passes
-- through on empty (C++ DoCast on nullptr casts — the burn
-- nil-skip was wrong, removed this run); the
-- InterruptNonMeleeSpells(false) arm is ported in kroshBlastWave.
-- All five: OnEnterCombat(1) resets per-GUID state, ports
-- DoZoneInCombat (creature:DoZoneInCombat, live bridge
-- engine/world/lua_creature_events.go, 2026-10-23; C++ arm order:
-- DoZoneInCombat first) and re-arms the combat timers (the
-- SetBossState(IN_PROGRESS) arms are blocked on the
-- instance-script model). OnDied(4): Maulgar Talk(SAY_DEATH);
-- the ogre JustDied relays port via relayOgreDeath (live-maulgar
-- Talk only); the SetBossState(DONE) arms are blocked — timers
-- are cancelled and per-GUID state dropped. OnTargetDied(3):
-- Maulgar Talk(
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
local SAY_OGRE_DEATH = 2
local SAY_SLAY = 3
local SAY_DEATH = 4

local timers = {}
local maulgarState = {}

-- C++ HealthBelowPct(pct) = GetHealth() < floor(maxHealth*pct/100)
-- (ironaya convention) — GetHealthPct() fires up to 1 HP early on the
-- float edge.
local function healthBelowPct(creature, pct)
    local max = creature:GetMaxHealth()
    if max == 0 then
        return false
    end
    return creature:GetHealth() < math.floor(max * pct / 100)
end

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

-- C++ SelectTarget(Random, 0) (playerOnly default FALSE, UnitAI.h:170)
-- — threat-list pick via the bridged, threat-ordered GetAITargets
-- (lua_creature_events.go:345); no Player filter (the midnight
-- randomTargetAnywhere precedent). Used by the three random-target
-- arms below (maulgar charge, olm death coil, kiggler polymorph).
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    if #targets == 0 then
        return nil
    end
    return targets[math.random(#targets)]
end

-- krosh blast wave (boss_high_king_maulgar.cpp:560): random
-- threat-list target within 15 yd (2D — IsWithinDist(me, 15,
-- false)) via GetAITargets, the threat-list bridge (same file's
-- charge / deathcoil / polymorph precedent). Distance rule kept
-- from the old player-pool twin: raw 2D dist minus the caster's
-- combat reach (target reach unmodeled, ~1.5yd band).
local function randomThreatTargetInRange2d(creature, range)
    local reach = creature:GetCombatReach()
    local candidates = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if creature:GetDistance2d(t) - reach < range then
            candidates[#candidates + 1] = t
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(1, #candidates)]
end

-- C++ ogre JustDied (boss_high_king_maulgar.cpp:263/350/442/524):
-- instance->GetGuidData(DATA_MAULGAR) -> maulgar->AI()->DoAction(
-- ACTION_ADD_DEATH) -> Talk(SAY_OGRE_DEATH 2). No DoAction bridge
-- exists, but the receiver lookup is bridged: FindNearestCreature
-- (svalna precedent) returns the live maulgar in the same
-- map+instance, nil when maulgar is dead or absent (the bridge
-- skips Health==0 entries).
local function relayOgreDeath(creature)
    local maulgar = creature:FindNearestCreature(NPC_MAULGAR, 40.0)
    if maulgar ~= nil then
        maulgar:Talk(SAY_OGRE_DEATH)
    end
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
    -- C++ rand32()%10000 exclusive top: [30000, 39999] (urand would be inclusive).
    schedule(guid, "mightyblow", math.random(30000, 39999), function()
        maulgarMightyBlow(creature, guid)
    end)
end

-- C++ phase 2 charging arm (boss_high_king_maulgar.cpp:183):
-- SelectTarget(Random, 0) — playerOnly FALSE (2-arg form) — random
-- target, DoCast(target, SPELL_BERSERKER_C); repeat 20s (initial 0s).
-- The AttackStart victim-switch arm has no bridge.
local function maulgarCharging(creature, guid)
    local target = randomThreatTarget(creature)
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
    -- C++ rand32()%10000 exclusive top: [40000, 49999] (urand would be inclusive).
    schedule(guid, "roar", math.random(40000, 49999), function()
        maulgarRoar(creature, guid)
    end)
end

-- C++ UpdateAI phase-2 gate, once-guarded: <50% health ->
-- Talk(SAY_ENRAGE) + triggered self-cast 29651 + the charging/
-- roar arms (the virtual-item-slot zeroing arms are ported in
-- maulgarPhaseCheck via creature:SetEquipmentSlots).
local function maulgarPhaseCheck(creature, guid)
    local state = maulgarState[guid]
    if state == nil or state.phase2 then
        return
    end
    -- C++ HealthBelowPct(50): strict-< against the floored integer
    -- threshold (the old GetHealthPct() < 50 fired up to 1 HP early).
    if healthBelowPct(creature, 50) then
        state.phase2 = true
        creature:Talk(SAY_ENRAGE)
        creature:CastSpell(creature, SPELL_DUAL_WIELD, true)
        -- C++ SetUInt32Value(UNIT_VIRTUAL_ITEM_SLOT_ID(+1), 0): clear the
        -- main-hand/off-hand display slots, slot 2 untouched (lua_creature_
        -- events.go SetEquipmentSlots; 0xFFFFFFFF = EQUIP_NO_CHANGE skip).
        creature:SetEquipmentSlots(0, 0, 0xFFFFFFFF)
        -- C++ phase-2 transition: Charging_Timer = 0, Roar_Timer = 0
        -- (next-tick fire; direct fire — CreateLuaEvent rejects delay 0,
        -- magmus/abomination precedent); both arms re-arm themselves.
        maulgarCharging(creature, guid)
        maulgarRoar(creature, guid)
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
-- and the phase-2 poller; DoZoneInCombat is PORTED (C++ arm order:
-- DoZoneInCombat first, before Talk) and the
-- SetBossState(IN_PROGRESS) arm is blocked. The AttackStart mutual
-- SetInCombatWith pairing (boss_high_king_maulgar.cpp:250-251) has no
-- bridge — engine triggerCreatureAggro converges the engage.
local function maulgarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    maulgarResetState(guid)
    maulgarState[guid] = { phase2 = false }
    creature:DoZoneInCombat() -- boss_high_king_maulgar.cpp:134: engage every player on the map (live bridge 2026-10-23)
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
    -- C++ Reset: non-triggered DoCast(me, SPELL_DUAL_WIELD) (Reset-only
    -- arm); ev23 fires on death too (engine reset-then-died order) but
    -- C++ Reset() never runs on death, so the cast is alive-gated.
    if creature:IsAlive() then
        creature:CastSpell(creature, SPELL_DUAL_WIELD)
    end
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

-- C++ DeathCoil_Timer (boss_high_king_maulgar.cpp:293):
-- SelectTarget(Random, 0) — playerOnly FALSE (2-arg form);
-- DoCast(target, SPELL_DEATH_COIL); repeat 20s (initial 20s).
local function olmDeathCoil(creature, guid)
    local target = randomThreatTarget(creature)
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

-- C++ JustEngagedWith: DoZoneInCombat PORTED (C++ arm order:
-- DoZoneInCombat first; boss_high_king_maulgar.cpp:257) +
-- SetBossState(IN_PROGRESS) (blocked) + the three combat
-- arms.
local function olmEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    olmResetState(guid)
    creature:DoZoneInCombat()
    olmArmCombat(creature, guid)
end

local function olmLeaveCombat(event, creature)
    olmResetState(creature:GetGUID())
end

-- C++ JustDied: the maulgar DoAction(ACTION_ADD_DEATH) relay
-- ports via relayOgreDeath above; the SetBossState(DONE) arm is
-- blocked on the instance-script model — timer cleanup only
-- besides the relay.
local function olmDied(event, creature, killer)
    relayOgreDeath(creature)
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

-- C++ GreaterPolymorph_Timer (boss_high_king_maulgar.cpp:366):
-- SelectTarget(Random, 0) — playerOnly FALSE (2-arg form); DoCast(target,
-- SPELL_GREATER_POLYMORPH); repeat urand(15000, 20000).
local function kigglerPolymorph(creature, guid)
    local target = randomThreatTarget(creature)
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
    creature:DoZoneInCombat() -- boss_high_king_maulgar.cpp:344, C++ arm order
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

-- C++ JustDied: the maulgar DoAction relay ports via
-- relayOgreDeath above; SetBossState(DONE) is blocked — timer
-- cleanup only besides the relay.
local function kigglerDied(event, creature, killer)
    relayOgreDeath(creature)
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
    creature:DoZoneInCombat() -- boss_high_king_maulgar.cpp:436, C++ arm order
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

-- C++ JustDied: the maulgar DoAction relay ports via
-- relayOgreDeath above; SetBossState(DONE) is blocked — timer
-- cleanup only besides the relay.
local function blindeyeDied(event, creature, killer)
    relayOgreDeath(creature)
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

-- C++ greater fireball (boss_high_king_maulgar.cpp:538-543):
-- fires when the timer lapses OR the victim is within 30 yd
-- (3D); both arms re-arm 2000. The 2s floor drives the timer
-- arm; a 500ms distance poller drives the early-fire arm —
-- an early fire restarts both the floor and the poller.
-- Deviation: Go has no UNIT_STATE_CASTING model, so in-range
-- casts fire at poll cadence rather than the C++ per-tick rate
-- (bounded in C++ by the cast-time gate) — documented.
local kroshFireballPoll
local function kroshFireball(creature, guid)
    creature:CastSpell(nil, SPELL_GREATER_FIREBALL)
    schedule(guid, "fireball", 2000, function()
        kroshFireball(creature, guid)
    end)
    schedule(guid, "fireballpoll", 500, function()
        kroshFireballPoll(creature, guid)
    end)
end

kroshFireballPoll = function(creature, guid)
    local victim = creature:GetVictim()
    if victim and creature:IsWithinDist(victim, 30) then
        kroshFireball(creature, guid)
    else
        schedule(guid, "fireballpoll", 500, function()
            kroshFireballPoll(creature, guid)
        end)
    end
end

-- C++-exact: InterruptNonMeleeSpells(false) + DoCastVictim(33054).
local function kroshSpellShield(creature, guid)
    creature:InterruptNonMeleeSpells(false)
    creature:CastSpell(nil, SPELL_SPELLSHIELD)
    schedule(guid, "spellshield", 30000, function()
        kroshSpellShield(creature, guid)
    end)
end

-- C++ blast wave (boss_high_king_maulgar.cpp:553-570): random
-- threat-list target within 15 yd (2D — IsWithinDist(me, 15,
-- false) at line 560) via GetAITargets (the threat-list bridge;
-- the old instance-player pool was the thaurissan bug class).
-- C++ casts on the pick even when the list is empty (DoCast on
-- nullptr is not a UnitAI early-out), so the pick passes through
-- rather than being nil-skipped. The InterruptNonMeleeSpells(false)
-- arm is ported below.
local function kroshBlastWave(creature, guid)
    local target = randomThreatTargetInRange2d(creature, 15)
    creature:InterruptNonMeleeSpells(false)
    creature:CastSpell(target, SPELL_BLAST_WAVE)
    schedule(guid, "blastwave", 60000, function()
        kroshBlastWave(creature, guid)
    end)
end

local function kroshResetState(guid)
    cancelTimers(guid)
end

local function kroshEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_high_king_maulgar.cpp:533: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    local guid = creature:GetGUID()
    kroshResetState(guid)
    creature:DoZoneInCombat() -- boss_high_king_maulgar.cpp:519, C++ JustEngagedWith arm order
    schedule(guid, "fireball", 1000, function()
        kroshFireball(creature, guid)
    end)
    -- C++: the distance OR runs on every UpdateAI tick from combat
    -- start, so a victim within 30 yd at pull eats a first-tick
    -- fireball (boss_high_king_maulgar.cpp:539). The poller starts
    -- at pull, not at the first floor fire; schedule() replaces the
    -- same-named arm, so an early fire supersedes the 1s floor arm
    -- exactly like the single C++ timer re-armed on either arm.
    schedule(guid, "fireballpoll", 1, function()
        kroshFireballPoll(creature, guid)
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

-- C++ JustDied: the maulgar DoAction relay ports via
-- relayOgreDeath above; SetBossState(DONE) is blocked — timer
-- cleanup only besides the relay.
local function kroshDied(event, creature, killer)
    relayOgreDeath(creature)
    kroshResetState(creature:GetGUID())
end

local function kroshReset(event, creature)
    kroshResetState(creature:GetGUID())
end

RegisterCreatureEvent(NPC_KROSH_FIREHAND, 1, kroshEnterCombat)
RegisterCreatureEvent(NPC_KROSH_FIREHAND, 2, kroshLeaveCombat)
RegisterCreatureEvent(NPC_KROSH_FIREHAND, 4, kroshDied)
RegisterCreatureEvent(NPC_KROSH_FIREHAND, 23, kroshReset)
