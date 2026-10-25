-- Priestess Delrissa (Magister's Terrace) --
-- Lua port of src/server/scripts/EasternKingdoms/MagistersTerrace/
-- boss_priestess_delrissa.cpp
-- (boss_priestess_delrissa — boss class only; the file's eight lackey
-- CreatureScript classes (boss_kagani_nightstrike, boss_ellris_
-- duskhallow, boss_eramas_brightblaze, boss_yazzai, boss_warlord_
-- salaris, boss_garaxxas, boss_apoko, boss_zelfan) plus the commented-
-- out npc_high_explosive_sheep are queued for later runs — lackey
-- machine parts (lackey GUID lists, threat adds, instance calls) have
-- no bridges here). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (boss_vexallus done: boss
-- class ported, entry 24744 registered; AddSC_boss_priestess_delrissa
-- next in the Magister's Terrace block; instance_magisters_terrace.cpp
-- stays blocked on the instance-script model).
-- Entry (verifiable from the C++ sources): magisters_terrace.h names
-- BOSS_PRIESTESS_DELRISSA = 24560 in the MTCreatureIds enum. The AI
-- is ScriptedAI (not BossAI) driven by DATA_PRIESTESS_DELRISSA = 2
-- via GetMagistersTerraceAI; the ported arms carry no instance state.
-- The creature_template ScriptName binding is DB-side (no TDB in this
-- workspace), but the entry itself is C++-verifiable so the port is
-- registered (boss_felblood_kaelthas precedent). Script name is the
-- stringified class name (CreatureScript("boss_priestess_delrissa");
-- RegisterCreatureAIWithFactory convention, felblood precedent).
-- Verifiable numbers (file's own enums/header): SAY_AGGRO = 0 /
-- SAY_DEATH = 10; LackeyDeath ids 1..4; PlayerDeath ids 5..9 /
-- SPELL_DISPEL_MAGIC = 27609 / SPELL_FLASH_HEAL = 17843 /
-- SPELL_SW_PAIN_NORMAL = 14032 / SPELL_SW_PAIN_HEROIC = 15654 /
-- SPELL_SHIELD = 44291 / SPELL_RENEW_NORMAL = 44174 /
-- SPELL_RENEW_HEROIC = 46192 / MAX_ACTIVE_LACKEY = 4 /
-- DATA_PRIESTESS_DELRISSA = 2 (MTDataTypes) /
-- DATA_DELRISSA_DEATH_COUNT = 5.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied (C++ Eluna::KilledUnit — lua_creature_events.go fires it
-- with (event, creature, victim) from Unit::Kill), 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith + UpdateAI heal/dispel/shield timers
-- + KilledUnit + JustDied):
-- Combat entry: Talk(SAY_AGGRO) on OnEnterCombat(1) (maiden-of-virtue
-- convention).
-- Heal machine: flash-heal 17843 15000ms -> 15000ms loop, PORTED with
-- the lackey leg: C++ (boss_priestess_delrissa.cpp:268-281) starts
-- target = me and replaces it with every alive lackey whose health is
-- below DELRISSA's health (the threshold is never updated in the
-- loop, so the LAST qualifying lackey wins — the old "least health"
-- paraphrase was wrong); the port re-finds each chosen lackey by
-- entry (FindNearestCreature, live snapshot) and casts on the final
-- target, falling back to self when no lackey qualifies.
-- Renew machine: renew 44174 10000ms -> 5000ms loop, PORTED with the
-- random-lackey leg: C++ gates on urand(0, 1), then a random lackey
-- slot; an alive lackey takes the cast, else self (the C++ has NO
-- heroic 46192 leg at all — the enum entry is never referenced in
-- the source (SD "No Heroic support yet"), so the "dropped — no
-- difficulty bridge" claim was stale and is retracted; there is no
-- heroic arm to gate).
-- Shield machine: shield 44291 2000ms -> 7500ms loop, PORTED with the
-- random-lackey leg: C++ gates on urand(0, 1), then a random lackey
-- slot alive and WITHOUT the shield aura (creature:HasAura is a live
-- aura-map read); else self.
-- Dispel machine: dispel-magic 27609 7500ms -> 12000ms loop, PORTED
-- with the full C++ target chooser: urand(0, 1) -> random alive player
-- within 100yd, else urand(0, 1) -> self, else a random alive lackey;
-- no cast when the chooser lands on nothing (C++ null-target guard).
-- KilledUnit: via OnTargetDied(3); the C++ victim->GetTypeId() ==
-- TYPEID_PLAYER gate is bridged by victim:GetObjectType() == "Player"
-- (selin precedent): Talk(PlayerDeath[PlayersKilled]) with a per-guid
-- counter incremented after each talk and capped at 4, i.e. ids
-- 5,6,7,8,9,9,9... (C++-exact: Talk then ++PlayersKilled only when
-- PlayersKilled < 4). The counter resets on OnEnterCombat(1) (C++
-- Initialize runs in the constructor and Reset — per-engagement reset
-- is the gyth/vaelastrasz convention).
-- JustDied: Talk(SAY_DEATH) on OnDied(4) (najentus/halycon precedent).
-- UpdateAI's !UsedPotion && HealthBelowPct(25) -> self-cast healing
-- potion 15503, once (boss_priestess_delrissa.cpp:452) — modeled on
-- OnDamageTaken(9) with the postDamage > 0 guard (nefarian rule) and a
-- per-guid latch cleared on 2/4/23 (C++-exact).
-- Timers start on OnEnterCombat(1), cancelled on OnLeaveCombat(2)/
-- OnDied(4)/OnReset(23) (maiden convention).
-- Deviations from C++: no UNIT_STATE_CASTING model in Go (casts are
-- packet-visual), so the timers fire unconditionally and the in-loop
-- casting gates are dropped (maiden precedent). The C++ scheduler is
-- driven from JustEngagedWith and drained by UpdateAI; the port
-- schedules from OnEnterCombat(1) and cancels on 2/4/23 (maiden
-- convention).
-- Reset() _Reset() / JustDied() _JustDied() are internal ScriptedAI
-- machinery covered by the cancel/re-arm on combat events (halycon
-- precedent); Reset's InitializeLackeys() summon/resummon machine is
-- PORTED on event 23 (below).
-- Unmodeled (documented-only, no bridges on the Lua surface):
-- - Reset()'s InitializeLackeys(): PORTED on event 23 — 4-of-8 random
--   selection + SummonCreature at the slot locations (type 4
--   CORPSE_DESPAWN) + the resummon-on-missing leg via
--   FindNearestCreature liveness (garaxxas precedent; dead-but-
--   undespawned corpses read as missing, the one divergence window).
-- - JustReachedHome: instance->SetBossState(DATA_PRIESTESS_DELRISSA,
--   FAIL) — no instance bridge (standing blocker).
-- - JustEngagedWith's lackey AddThreat(who, 0.0f, pAdd) ring: stays
--   out — the AddThreat bridge is self-threat only (the 3rd-arg
--   who-redirect is not expressible, lua_creature_events.go:581),
--   AttackStart is not on the creature surface, and DoZoneInCombat
--   engages only the caller; no bridge makes ANOTHER creature engage.
--   The SetBossState IN_PROGRESS arm — instance bridge.
-- - JustDied's DATA_DELRISSA_DEATH_COUNT check and the lootable-flag
--   manipulation — instance bridge.
-- - SW_PAIN machine (PORTED): DoCast(SelectTarget(Random, 0, 100,
--   true), SPELL_SW_PAIN_NORMAL 14032) 5000ms init -> 10000ms loop —
--   random-player-in-100yd port, re-arms unconditionally (C++ re-arms
--   outside the null-target guard). There is NO heroic 15654 leg in
--   the C++ — the enum entry is never referenced (SD "No Heroic
--   support yet"), so the "dropped (no difficulty bridge)" claim was
--   stale and is retracted.
-- - ResetTimer's home-position Z+10 evade check (EnterEvadeMode) —
--   GetHomePosition IS bridged (lua_creature_events.go:1475) but EnterEvadeMode
--   has no bridge (boss_ai.go:5753-5754 explicit) — the check stays out
--   (the_beast evade precedent).
-- - The eight lackey CreatureScript classes + commented-out
--   npc_high_explosive_sheep: each lackey inherits
--   boss_priestess_lackey_commonAI (AcquireGUIDs / threat-reset timer
--   / JustDied death-count talk to Delrissa / KilledUnit forward /
--   sub-25% healing-potion latch / Reset respawn of Delrissa) and adds
--   its own spell machine. The bridgeable slices are now PORTED in
--   per-class files (entries from the file's own m_auiAddEntries,
--   line 104): 24554 Eramas Brightblaze (knockdown 11428, snap-kick
--   46182, potion latch), 24557 Kagani Nightstrike (gouge 12540, kick
--   27613, eviscerate 27611, potion latch), 24558 Elris Duskhallow
--   (immolate 44267, shadow-bolt 12471, potion latch), 24561 Yazzaj
--   (ice-lance 46194, cone-of-cold 38384, frostbolt 15043, ice-block
--   27619 sub-35% latch, potion latch), 24559 Warlord Salaris (battle-
--   shout 27578 JustEngagedWith self-cast, disarm 27581, hamstring
--   27584, mortal-strike 44268, piercing-howl 23600, frightening-shout
--   19134, potion latch), 24555 Garaxxas (aimed-shot 44271, shoot
--   15620, concussive-shot 27634, multi-shot 31942, wing-clip 44286,
--   potion latch), 24553 Apoko (totem roulette RAND(27621/44257/15786),
--   war-stomp 46026, frost-shock 21401, lesser-healing-wave 44256,
--   potion latch), 24556 Zelfan (goblin-dragon-gun 44272, rocket-launch
--   44137, fel-iron-bomb 46024, high-explosive-sheep 44276, potion
--   latch). "Bridgeable later" was stale — the victim-loop / self-
--   cast / latch arms needed no new bridges (CreateLuaEvent timer
--   convention).
--   Genuinely staying out per lackey (no bridges): Kagani vanish
--   machine (SelectTarget(Random,0) non-playerOnly + threat legs);
--   Ellris summon-imp 44163 (summon), seed-of-corruption 44141 /
--   curse-of-agony 14875 / fear 38595 (SelectTarget Random,0);
--   Yazzai polymorph 13323 / blizzard 44178 (SelectTarget Random,0),
--   blink 14514 (combat-manager any-combatant melee-range check —
--   victim-distance proxy would fabricate targeting); Salaris
--   intercept 27577 (melee-range iteration + SelectTarget Random,0);
--   Garaxxas freezing-trap 44136 (one-trap-at-a-time GO check),
--   sliver pet 24552 (summon); Zelfan recombobulate 44274 (lackey-
--   GUID-list + polymorph check); all common-AI instance/threat/
--   GUID-list arms. npc_high_explosive_sheep is commented out in C++
--   — nothing to port.
-- - Apoko (shaman): totem roulette RAND(27621/44257/15786) self-cast,
--   war-stomp 46026 self-cast, frost-shock 21401 victim, lesser-
--   healing-wave 44256 self (bridgeable later); purge 27626 —
--   SelectTarget bridge.
-- - Zelfan (engineer): goblin-dragon-gun 44272 / rocket-launch 44137 /
--   fel-iron-bomb 46024 victim loops + high-explosive-sheep 44276
--   self-cast (bridgeable later); recombobulate 44274 — lackey GUID
--   list + IsPolymorphed bridges.
local SAY_AGGRO = 0
local SAY_DEATH = 10

local SPELL_FLASH_HEAL = 17843
local SPELL_RENEW_NORMAL = 44174
local SPELL_SHIELD = 44291
local SPELL_DISPEL_MAGIC = 27609
local SPELL_SW_PAIN = 14032
local SPELL_HEALING_POTION = 15503

local PlayerDeath = { 5, 6, 7, 8, 9 }
local MAX_PLAYER_DEATH_TALK = 4

local ENTRY_PRIESTESS_DELRISSA = 24560

-- C++ InitializeLackeys (boss_priestess_delrissa.cpp:178-230): 4 of the
-- 8 m_auiAddEntries summoned at LackeyLocations, fZLocation -19.921,
-- fOrientation 4.98, TEMPSUMMON_CORPSE_DESPAWN (Go summon type 4).
local MAX_ACTIVE_LACKEY = 4
local LACKEY_ENTRIES = { 24557, 24558, 24554, 24561, 24559, 24555, 24553, 24556 }
local LACKEY_LOCATIONS = { { 123.77, 17.6007 }, { 131.731, 15.0827 }, { 121.563, 15.6213 }, { 129.988, 17.2355 } }
local LACKEY_Z = -19.921
local LACKEY_O = 4.98
local SUMMON_TYPE_CORPSE_DESPAWN = 4
local LACKEY_FIND_RANGE = 100.0

local timers = {}
local playersKilled = {}
local usedPotion = {}
local lackeyEntries = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0, 100, true)
-- (playerOnly): random alive threat-list player within 100 yd
-- (beast/voshgajin precedent).
local function randomThreatPlayerInRange(creature, maxDist)
    local found = {}
    for _, t in ipairs(creature:GetAITargets()) do
        if t:GetObjectType() == "Player" and not t:IsDead()
                and creature:GetDistance(t) <= maxDist then
            found[#found + 1] = t
        end
    end
    if #found == 0 then
        return nil
    end
    return found[math.random(#found)]
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

-- C++ InitializeLackeys first branch: copy all 8 entries, erase random
-- entries until 4 remain (sequential random erasure, C++-exact
-- distribution), then summon the survivors in list order at the slot
-- locations. Later Resets run the resummon leg: a slot whose entry has
-- no live creature in range is re-summoned. The live check is
-- FindNearestCreature (garaxxas precedent): it skips Health == 0, so a
-- dead-but-not-yet-despawned lackey corpse reads as missing where C++
-- GetUnit would still see it — the one known divergence window.
-- SummonCreature returns the summoned object, but stored objects go
-- stale (luaMotionCreature snapshots state at build), so every leg
-- below re-finds its lackey fresh by entry instead of trusting a stored
-- reference; the per-GUID state kept here is the chosen entry list
-- only, mirroring C++'s LackeyEntryList (cleared only in the ctor, so
-- it survives death/evade like the C++ list).
local function initializeLackeys(creature, guid)
    if creature:IsDead() then
        return
    end
    local entries = lackeyEntries[guid]
    if not entries then
        local pool = {}
        for i, entry in ipairs(LACKEY_ENTRIES) do
            pool[i] = entry
        end
        while #pool > MAX_ACTIVE_LACKEY do
            table.remove(pool, math.random(#pool))
        end
        entries = pool
        lackeyEntries[guid] = entries
        for i, entry in ipairs(entries) do
            local loc = LACKEY_LOCATIONS[i]
            creature:SummonCreature(entry, loc[1], loc[2], LACKEY_Z, LACKEY_O, SUMMON_TYPE_CORPSE_DESPAWN)
        end
    else
        for i, entry in ipairs(entries) do
            if not creature:FindNearestCreature(entry, LACKEY_FIND_RANGE) then
                local loc = LACKEY_LOCATIONS[i]
                creature:SummonCreature(entry, loc[1], loc[2], LACKEY_Z, LACKEY_O, SUMMON_TYPE_CORPSE_DESPAWN)
            end
        end
    end
end

local function randomLackey(creature, guid)
    local entries = lackeyEntries[guid]
    if not entries then
        return nil
    end
    return creature:FindNearestCreature(entries[math.random(#entries)], LACKEY_FIND_RANGE)
end

local function onHeal(creature, guid)
    local target = creature
    local myHealth = creature:GetHealth()
    local entries = lackeyEntries[guid]
    if entries then
        for _, entry in ipairs(entries) do
            local lackey = creature:FindNearestCreature(entry, LACKEY_FIND_RANGE)
            if lackey and not lackey:IsDead() and lackey:GetHealth() < myHealth then
                target = lackey
            end
        end
    end
    creature:CastSpell(target, SPELL_FLASH_HEAL)
    schedule(guid, "heal", 15000, function() onHeal(creature, guid) end)
end

local function onRenew(creature, guid)
    local target = creature
    if math.random(0, 1) == 1 then
        local lackey = randomLackey(creature, guid)
        if lackey and not lackey:IsDead() then
            target = lackey
        end
    end
    creature:CastSpell(target, SPELL_RENEW_NORMAL)
    schedule(guid, "renew", 5000, function() onRenew(creature, guid) end)
end

local function onShield(creature, guid)
    local target = creature
    if math.random(0, 1) == 1 then
        local lackey = randomLackey(creature, guid)
        if lackey and not lackey:IsDead() and not lackey:HasAura(SPELL_SHIELD) then
            target = lackey
        end
    end
    creature:CastSpell(target, SPELL_SHIELD)
    schedule(guid, "shield", 7500, function() onShield(creature, guid) end)
end

local function onDispel(creature, guid)
    local target = nil
    if math.random(0, 1) == 1 then
        target = randomThreatPlayerInRange(creature, 100)
    else
        if math.random(0, 1) == 1 then
            target = creature
        else
            local lackey = randomLackey(creature, guid)
            if lackey and not lackey:IsDead() then
                target = lackey
            end
        end
    end
    if target then
        creature:CastSpell(target, SPELL_DISPEL_MAGIC)
    end
    schedule(guid, "dispel", 12000, function() onDispel(creature, guid) end)
end

local function onSwPain(creature, guid)
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_SW_PAIN)
    end
    schedule(guid, "swpain", 10000, function() onSwPain(creature, guid) end)
end

local function onReset(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    usedPotion[guid] = nil
    playersKilled[guid] = nil
    initializeLackeys(creature, guid)
end

local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    playersKilled[guid] = 0
    usedPotion[guid] = false
    creature:Talk(SAY_AGGRO)
    schedule(guid, "heal", 15000, function() onHeal(creature, guid) end)
    schedule(guid, "renew", 10000, function() onRenew(creature, guid) end)
    schedule(guid, "shield", 2000, function() onShield(creature, guid) end)
    schedule(guid, "dispel", 7500, function() onDispel(creature, guid) end)
    schedule(guid, "swpain", 5000, function() onSwPain(creature, guid) end)
end

local function onCombatEnd(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    usedPotion[guid] = nil
    playersKilled[guid] = nil
end

-- C++ UpdateAI (boss_priestess_delrissa.cpp:452): !UsedPotion &&
-- HealthBelowPct(25) -> DoCast(me, SPELL_HEALING_POTION 15503),
-- once. The postDamage > 0 guard (nefarian rule): the C++ UpdateAI
-- arm never fires on the killing blow. Integer floor threshold
-- (C++-exact), per-guid latch cleared on 2/4/23.
local function onDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUID()
    if usedPotion[guid] then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = creature:GetHealth() - damage
    if postDamage > 0 and postDamage < math.floor(maxHealth * 25 / 100) then
        usedPotion[guid] = true
        creature:CastSpell(creature, SPELL_HEALING_POTION)
    end
end

local function onTargetDied(_, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        local guid = creature:GetGUID()
        local n = playersKilled[guid] or 0
        if n > MAX_PLAYER_DEATH_TALK then
            n = MAX_PLAYER_DEATH_TALK
        end
        creature:Talk(PlayerDeath[n + 1])
        if n < MAX_PLAYER_DEATH_TALK then
            playersKilled[guid] = n + 1
        end
    end
end

-- C++ boss_priestess_delrissaAI::JustDied :417 instance->SetData(DATA_DELRISSA_DEATH_COUNT, SPECIAL):
-- the only readers are the unported lackey AIs (boss_priestess_lackey_commonAI :242/:409) — no
-- instance SetData/GetData bridge, and no cross-AI bridge to the unported lackeys. Dropped, documented.
local function onDied(_, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    usedPotion[guid] = nil
    playersKilled[guid] = nil
    creature:Talk(SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 3, onTargetDied)
RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 4, onDied)
RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 9, onDamageTaken)
RegisterCreatureEvent(ENTRY_PRIESTESS_DELRISSA, 23, onReset)
