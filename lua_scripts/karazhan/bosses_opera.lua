-- The Opera Event (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/bosses_opera.cpp (1562 lines:
-- Wizard of Oz, Red Riding Hood, and Romulo & Julianne).
-- Creature entries (verified): 17535 Dorothee (wowhead mop-classic
-- npc=17535/dorothee), 17543 Strawman (wowhead npc=17543/strawman),
-- 17547 Tinhead (wowhead wotlk npc=17547/tinhead), 17546 Roar (wowhead
-- mop-classic npc=17546/roar), 17548 Tito (C++ CREATURE_TITO), 18168 the
-- Crone (C++ CREATURE_CRONE), 18412 Cyclone (C++ CREATURE_CYCLONE),
-- 17603 Grandmother (wowhead npc=17603/grandmother), 17521 the Big Bad Wolf
-- (C++ CREATURE_BIG_BAD_WOLF; wowhead npc=17521/the-big-bad-wolf),
-- 17534 Julianne (wowhead wotlk npc=17534/julianne), 17533 Romulo
-- (C++ CREATURE_ROMULO). C++ ScriptNames per AddSC_bosses_opera:
-- boss_dorothee, boss_strawman, boss_tinhead, boss_roar, boss_crone,
-- npc_tito, npc_cyclone, npc_grandmother, boss_bigbadwolf, boss_julianne,
-- boss_romulo.
-- Instance data: DATA_OPERA_PERFORMANCE = 4, DATA_OPERA_OZ_DEATHCOUNT = 14
-- (karazhan.h:34,43); every instance arm has no Go model (see deviations).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken (the Julianne/Romulo fake-death dance),
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Deviations from C++: Tito (17548), the Crone (SummonCroneIfReady),
-- Romulo (17533), and the wolf (grandmother gossip, 17603) ARE summoned
-- now (2026-10-22 summon bridge — creature:SummonCreature on the handler
-- surface); the cyclone summon still has no bearer. The C++ aggro legs
-- (AttackStart / Attack have no bridge; DoZoneInCombat is bridged since
-- 2026-10-23 and Romulo's summon-leg DoZoneInCombat(pRomulo) IS ported
-- in the SummonRomuloTimer leg below — receiver-agnostic bridge, the
-- summon object in hand; other summon legs still idle until the engine
-- aggros them), so most summons idle
-- until the engine aggros them — npc_tito (17548), boss_crone (18168),
-- boss_bigbadwolf (17521) and boss_romulo (17533) are all registered, so
-- a spawned one fights correctly once engaged (aran-elemental
-- convention). No
-- instance-script model — the Oz death counter, DATA_OPERA_PERFORMANCE
-- SetBossState(DONE), and DoZoneInCombat arms are skipped
-- (instance-legs: re-evaluated 2026-10-23, stays out). No threat
-- model — the wolf's chase threat juggling is skipped (chase is Talk +
-- debuff + a fear/swipe freeze) and BackwardLunge picks a random alive
-- player excluding the victim (approximates the C++ second-threat
-- offset-1 pick; re-arm-on-success + next-tick retry mirror the C++
-- timer semantics, yazzai-Polymorph precedent). The Oz AggroTimer
-- RemoveFlag(NON_ATTACKABLE) arms are PORTED (2026-10-20) via
-- creature:RemoveFlag(UNIT_FIELD_FLAGS); PretendToDie's
-- NOT_SELECTABLE/SetHealth(0)/stand-state arms stay out (fake death is
-- timer-cancel plus shared flags); Resurrect's flag/health/stand-state
-- restores are PORTED (RemoveFlag + SetHealth(GetMaxHealth()) +
-- SetStandState(0) ahead of the res visual, C++ Resurrect order).
-- No UNIT_STATE_CASTING model — timers
-- fire unconditionally (InterruptNonMeleeSpells skipped). No facing model —
-- BackwardLunge's behind-target (HasInArc) gate is ported via the inArc twin. The
-- strawman fire-school SpellHit arm (Burning Straw 31075) is skipped —
-- event 14 carries only the spell id, no school mask. The crone's
-- RemoveFlag/SetImmuneToPC spawn hack is PORTED in croneLeaveCombat. npc_cyclone (18412)
-- is not registered: movement-only AI, no combat hooks ever fire.
-- npc_grandmother (17603) IS registered now (2026-10-22 summon bridge):
-- the gossip select summons the Big Bad Wolf at her position and despawns
-- her, C++-exact; the wolf's AttackStart(player) leg has no bridge
-- (documented gap). The wolf's
-- death sound (9275) is ported via creature:PlayDirectSound in
-- wolfDied (SMSG_PLAY_SOUND = WorldObject::PlayDirectSound). Julianne's and
-- Romulo's pre-combat EntryYell/AggroYell timers have no Lua-modelable
-- pre-combat tick (their NON_ATTACKABLE/faction arms have no bridge
-- either). Julianne's SpellHit(DRINK_POISON) arm is folded into the
-- event-9 hook — the only caster is her own event-9 arm, so the id-only
-- spell-hit hook adds nothing. PretendToDie's timer freeze is ported via
-- deadline banking (the wolf fear/swipe pattern): the combat arms record
-- fire-times through rajSchedule, pretend-die banks the remainders before
-- cancelling, and resurrect re-arms at the banked remainder (0 -> 1ms, so
-- a zero remainder fires next tick) or the combat-initial delay when
-- nothing was banked — matching C++ UpdateAI resuming the untouched timers
-- on IsFakingDeath exit. The partner's
-- real-death cross-kill: the setDeathState(JUST_DIED) / MotionMaster legs have no bridge;
-- CombatStop(true) is PORTED at the C++ position (before the LOOTABLE flag) — the survivor's OnDied runs while
-- the faking partner stays down — but the flag legs ARE ported: the
-- partner's faking body gets UNIT_DYNFLAG_LOOTABLE via the
-- creature:SetFlag(79) dynamic-flag bridge (lua_creature_events.go) at
-- the C++ arm position, and the partner's pending resurrect timer is
-- cancelled there (a dead AI ticks nothing in C++). The
-- Romulo↔Julianne dance shares one module-level table (single-instance
-- approximation; C++ keeps per-AI copies that agree in the normal flow),
-- and cross-creature resurrects only fire for a partner that has entered
-- combat (no ObjectAccessor bridge).

local ENTRY_DOROTHEE = 17535
local ENTRY_TITO = 17548
local ENTRY_STRAWMAN = 17543
local ENTRY_TINHEAD = 17547
local ENTRY_ROAR = 17546
local ENTRY_CRONE = 18168
local ENTRY_WOLF = 17521
local ENTRY_GRANDMOTHER = 17603
local ENTRY_JULIANNE = 17534
local ENTRY_ROMULO = 17533

local timers = {}

-- UnitDefines.h:125; RemoveFlag on UNIT_FIELD_FLAGS (59) is bridged
-- (engine/world/lua_creature_events.go:725).
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000

-- Oz intro timers: the AggroTimer arms (bosses_opera.cpp) are kept out
-- of the shared timer keyspace — C++ never cancels AggroTimer on
-- engage (the UpdateAI arm runs in combat), and Reset restarts it.
local introTimers = {}

local function cancelIntroTimer(guid)
    local id = introTimers[guid]
    if id then
        RemoveEventById(id)
        introTimers[guid] = nil
    end
end

local function scheduleIntroReset(guid, creature, delay)
    cancelIntroTimer(guid)
    introTimers[guid] = CreateLuaEvent(function()
        introTimers[guid] = nil
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    end, delay)
end

-- C++ SummonCroneIfReady (bosses_opera.cpp:104-114): free function called
-- from the JustDied arms of Dorothee, Strawman, Tinhead and Roar. The
-- instance DATA_OPERA_OZ_DEATHCOUNT counter has no Go model; a
-- module-level counter mirrors it (single-instance convention, like the
-- raj table below). C++ resets it only in npc_barnes' StartEvent
-- (karazhan.cpp:179), which is unported, so the Lua counter likewise
-- never resets. At 4 deaths the Crone (18168) is summoned at
-- (-10891.96, -1755.95, my z, 4.64), TEMPSUMMON_TIMED_OR_DEAD_DESPAWN
-- (0), 2h = 7200000ms. C++ pCrone->AI()->AttackStart(victim) has no
-- bridge — the boss_crone AI below is registered, so she fights once
-- engaged (documented gap).
local ozDeathCount = 0

local function summonCroneIfReady(creature)
    ozDeathCount = ozDeathCount + 1
    if ozDeathCount == 4 then
        creature:SummonCreature(ENTRY_CRONE, -10891.96, -1755.95,
            creature:GetZ(), 4.64, 0, 7200000)
    end
end

-- Tito <-> Dorothee link (C++ npc_titoAI::DorotheeGUID, written by
-- SummonTito): titoGuid -> dorotheeGuid, plus the per-Dorothee TitoDied
-- latch (C++ boss_dorotheeAI::TitoDied) and her creature object for the
-- IsAlive gate in tito's JustDied.
local titoDorothee = {}
local dorotheeTitoDied = {}
local dorotheeByGuid = {}

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

local function cancelKeys(guid, keys)
    local per = timers[guid]
    if per then
        for _, key in ipairs(keys) do
            if per[key] then
                RemoveEventById(per[key])
                per[key] = nil
            end
        end
    end
end

-- C++ SelectTarget(Random, 0, 100, true) (bosses_opera.cpp:422 brain
-- wipe, :923 wolf chase, :1352 deadly swathe, :1456 blinding passion):
-- the pool is the threat list, not the instance players.

-- C++ SelectTarget(Random, 0) with playerOnly=FALSE (UnitAI.h:170):
-- random over the threat list, pets/totems eligible. Bridged via
-- creature:GetAITargets() (threat-ordered; lua_creature_events.go:345),
-- the boss_midnight randomTargetAnywhere precedent.
local function randomThreatTarget(creature)
    local targets = creature:GetAITargets()
    local alive = {}
    for _, t in ipairs(targets) do
        if not t:IsDead() then
            alive[#alive + 1] = t
        end
    end
    if #alive == 0 then
        return nil
    end
    return alive[math.random(#alive)]
end

local function randomPlayerInRange(creature, maxDist)
    local candidates = {}
    local reach = creature:GetCombatReach()
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - reach - 1.5 < maxDist then -- IsWithinCombatRange gate (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            candidates[#candidates + 1] = unit
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Dorothee (boss_dorothee, C++ ScriptName for 17535): water bolt 31012 on
-- a random alive player within 100 yd every 5s (first 5s; 1500ms once
-- Tito has died — the TitoDied latch, C++-exact); scream 31013 on the
-- victim every 30s (first 15s); Tito (17548) summoned at 47.5s with
-- Talk(SAY_DOROTHEE_SUMMON = 1).
local DOROTHEE_SAY_DEATH = 0
local DOROTHEE_SAY_SUMMON = 1
local DOROTHEE_SAY_TITO_DEATH = 2
local DOROTHEE_SAY_AGGRO = 3

local SPELL_WATERBOLT = 31012
local SPELL_SCREAM = 31013

local function dorotheeWaterbolt(creature, guid)
    -- C++ SelectTarget(Random, 0) (:216): playerOnly=FALSE (UnitAI.h:170)
    -- — the pool is the threat list (pets/totems eligible), not the map;
    -- no range cap; C++ SelectTargetList skips offline refs before
    -- the pick (UnitAI.h), so dead entries are filtered. Lowered over
    -- the threat-ordered GetAITargets list
    -- (lua_creature_events.go:345 — the midnight precedent).
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_WATERBOLT)
    end
    -- C++ WaterBoltTimer = TitoDied ? 1500 : 5000 (:218).
    schedule(guid, "waterbolt", dorotheeTitoDied[guid] and 1500 or 5000, function()
        dorotheeWaterbolt(creature, guid)
    end)
end

local function dorotheeFear(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SCREAM)
    end
    schedule(guid, "fear", 30000, function()
        dorotheeFear(creature, guid)
    end)
end

-- C++ boss_dorothee::boss_dorotheeAI::SummonTito
-- (bosses_opera.cpp:300-309): summon Tito (17548); C++ passes
-- all-zero coords, which ride the bridge's Object.cpp:2059-2060
-- close-point fallback (C++-exact). TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT (3),
-- 30s = 30000ms, then Talk(SAY_DOROTHEE_SUMMON). The ENSURE_AI
-- DorotheeGUID write is the titoDorothee module link above;
-- pTito->AI()->AttackStart(victim) has no bridge (documented gap).
local function dorotheeSummonTito(creature, guid)
    local tito = creature:SummonCreature(ENTRY_TITO, 0, 0, 0, 0, 3, 30000)
    if tito ~= nil then
        creature:Talk(DOROTHEE_SAY_SUMMON)
        titoDorothee[tito:GetGUID()] = guid
        dorotheeTitoDied[guid] = false
    end
end

local function dorotheeEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    dorotheeByGuid[guid] = creature
    dorotheeTitoDied[guid] = false
    creature:Talk(DOROTHEE_SAY_AGGRO)
    schedule(guid, "waterbolt", 5000, function()
        dorotheeWaterbolt(creature, guid)
    end)
    schedule(guid, "fear", 15000, function()
        dorotheeFear(creature, guid)
    end)
    -- C++ SummonTitoTimer = 47500 (Initialize, :142); the UpdateAI arm
    -- (!SummonedTito gate) fires it once.
    schedule(guid, "tito", 47500, function()
        dorotheeSummonTito(creature, guid)
    end)
end

local function dorotheeLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    dorotheeByGuid[guid] = nil
    dorotheeTitoDied[guid] = nil
end

-- C++ boss_dorotheeAI::UpdateAI AggroTimer (bosses_opera.cpp:204-208):
-- 0.5s after spawn/evade the template-set NON_ATTACKABLE flag is
-- cleared (AggroTimer=500 in Initialize, :138; no SetFlag in file).
local function dorotheeReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ Initialize (Reset): SummonedTito = false, TitoDied = false.
    dorotheeTitoDied[guid] = false
    scheduleIntroReset(guid, creature, 500)
end

local function dorotheeDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    dorotheeByGuid[guid] = nil
    dorotheeTitoDied[guid] = nil
    creature:Talk(DOROTHEE_SAY_DEATH)
    -- C++ JustDied arm order: Talk, then SummonCroneIfReady.
    summonCroneIfReady(creature)
end


-- C++ JustReachedHome() -> me->DespawnOrUnsummon() (bosses_opera.cpp);
-- event 24 = CreatureEventOnReachHome (runtime.go:207), Despawn bridged
-- (lua_creature_events.go:214).
local function dorotheeReachHome(event, creature)
    dorotheeLeaveCombat(event, creature)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_DOROTHEE, 1, dorotheeEnterCombat)
RegisterCreatureEvent(ENTRY_DOROTHEE, 2, dorotheeLeaveCombat)
RegisterCreatureEvent(ENTRY_DOROTHEE, 4, dorotheeDied)
RegisterCreatureEvent(ENTRY_DOROTHEE, 23, dorotheeReset)
RegisterCreatureEvent(ENTRY_DOROTHEE, 24, dorotheeReachHome)

-- Tito (npc_tito, C++ ScriptName for 17548): yipping 31015 on the victim
-- every 10s. Summoned by Dorothee at 47.5s (summon bridge, 2026-10-22);
-- registered so the summoned Tito fights correctly once engaged. The
-- JustDied -> Dorothee arm (TitoDied latch plus Tito
-- Talk(SAY_DOROTHEE_TITO_DEATH = 2), gated on the linked Dorothee being
-- alive via the titoDorothee module link) is ported below.
local SPELL_YIPPING = 31015

local function titoYip(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_YIPPING)
    end
    schedule(guid, "yip", 10000, function()
        titoYip(creature, guid)
    end)
end

local function titoEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "yip", 10000, function()
        titoYip(creature, guid)
    end)
end

local function titoLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_titoAI::JustDied (bosses_opera.cpp:271-281): if the linked
-- Dorothee (DorotheeGUID via ObjectAccessor) is alive, her TitoDied
-- latch sets — her waterbolt re-arms at 1500ms — and Tito Talks
-- SAY_DOROTHEE_TITO_DEATH.
local function titoDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local dorGuid = titoDorothee[guid]
    titoDorothee[guid] = nil
    local dorothee = dorGuid ~= nil and dorotheeByGuid[dorGuid] or nil
    if dorothee ~= nil and not dorothee:IsDead() then
        dorotheeTitoDied[dorGuid] = true
        creature:Talk(DOROTHEE_SAY_TITO_DEATH)
    end
end

RegisterCreatureEvent(ENTRY_TITO, 1, titoEnterCombat)
RegisterCreatureEvent(ENTRY_TITO, 2, titoLeaveCombat)
RegisterCreatureEvent(ENTRY_TITO, 4, titoDied)
RegisterCreatureEvent(ENTRY_TITO, 23, titoLeaveCombat)

-- Strawman (boss_strawman, C++ ScriptName for 17543): brain bash 31046 on
-- the victim every 15s (first 5s); brain wipe 31069 on a random alive
-- player within 100 yd every 20s (first 7s). The fire-school SpellHit arm
-- (burning straw 31075) has no bridge — event 14 carries only the spell
-- id. The AggroTimer NON_ATTACKABLE arm is PORTED (2026-10-20).
local STRAWMAN_SAY_AGGRO = 0
local STRAWMAN_SAY_DEATH = 1
local STRAWMAN_SAY_SLAY = 2

local SPELL_BRAIN_BASH = 31046
local SPELL_BRAIN_WIPE = 31069

local function strawmanBrainBash(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_BRAIN_BASH)
    end
    schedule(guid, "bash", 15000, function()
        strawmanBrainBash(creature, guid)
    end)
end

local function strawmanBrainWipe(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_BRAIN_WIPE)
    end
    schedule(guid, "wipe", 20000, function()
        strawmanBrainWipe(creature, guid)
    end)
end

local function strawmanEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(STRAWMAN_SAY_AGGRO)
    schedule(guid, "bash", 5000, function()
        strawmanBrainBash(creature, guid)
    end)
    schedule(guid, "wipe", 7000, function()
        strawmanBrainWipe(creature, guid)
    end)
end

local function strawmanLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_strawmanAI::UpdateAI AggroTimer (bosses_opera.cpp:402-408):
-- 13s after spawn/evade the template-set NON_ATTACKABLE flag is
-- cleared (AggroTimer=13000 in Initialize, :332; no SetFlag in file).
local function strawmanReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    scheduleIntroReset(guid, creature, 13000)
end

local function strawmanTargetDied(event, creature, victim)
    -- C++ KilledUnit Talks unconditionally (no TYPEID_PLAYER gate).
    if victim ~= nil then
        creature:Talk(STRAWMAN_SAY_SLAY)
    end
end

local function strawmanDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(STRAWMAN_SAY_DEATH)
    -- C++ JustDied arm order: Talk, then SummonCroneIfReady.
    summonCroneIfReady(creature)
end


-- C++ JustReachedHome() -> me->DespawnOrUnsummon() (bosses_opera.cpp);
-- event 24 = CreatureEventOnReachHome (runtime.go:207), Despawn bridged
-- (lua_creature_events.go:214).
local function strawmanReachHome(event, creature)
    strawmanLeaveCombat(event, creature)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_STRAWMAN, 1, strawmanEnterCombat)
RegisterCreatureEvent(ENTRY_STRAWMAN, 2, strawmanLeaveCombat)
RegisterCreatureEvent(ENTRY_STRAWMAN, 3, strawmanTargetDied)
RegisterCreatureEvent(ENTRY_STRAWMAN, 4, strawmanDied)
RegisterCreatureEvent(ENTRY_STRAWMAN, 23, strawmanReset)
RegisterCreatureEvent(ENTRY_STRAWMAN, 24, strawmanReachHome)

-- Tinhead (boss_tinhead, C++ ScriptName for 17547): cleave 31043 on the
-- victim every 5s; rust 31086 on self — first at 30s, then every 6s, at
-- most 8 rusts, each with Talk(EMOTE_RUST = 3).
local TINHEAD_SAY_AGGRO = 0
local TINHEAD_SAY_DEATH = 1
local TINHEAD_SAY_SLAY = 2
local TINHEAD_EMOTE_RUST = 3

local SPELL_CLEAVE = 31043
local SPELL_RUST = 31086

local tinheadState = {}

local function tinheadCleave(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CLEAVE)
    end
    schedule(guid, "cleave", 5000, function()
        tinheadCleave(creature, guid)
    end)
end

local function tinheadRust(creature, guid)
    local st = tinheadState[guid]
    if st == nil or st.rustCount >= 8 then
        return
    end
    st.rustCount = st.rustCount + 1
    creature:Talk(TINHEAD_EMOTE_RUST)
    creature:CastSpell(creature, SPELL_RUST)
    schedule(guid, "rust", 6000, function()
        tinheadRust(creature, guid)
    end)
end

local function tinheadEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    tinheadState[guid] = { rustCount = 0 }
    creature:Talk(TINHEAD_SAY_AGGRO)
    schedule(guid, "cleave", 5000, function()
        tinheadCleave(creature, guid)
    end)
    schedule(guid, "rust", 30000, function()
        tinheadRust(creature, guid)
    end)
end

local function tinheadLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    tinheadState[guid] = nil
end

-- C++ boss_tinheadAI::UpdateAI AggroTimer (bosses_opera.cpp:515-519):
-- 15s after spawn/evade the template-set NON_ATTACKABLE flag is
-- cleared (AggroTimer=15000 in Initialize, :452; no SetFlag in file).
local function tinheadReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    tinheadState[guid] = nil
    scheduleIntroReset(guid, creature, 15000)
end

local function tinheadTargetDied(event, creature, victim)
    -- C++ KilledUnit Talks unconditionally (no TYPEID_PLAYER gate).
    if victim ~= nil then
        creature:Talk(TINHEAD_SAY_SLAY)
    end
end

local function tinheadDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    tinheadState[guid] = nil
    creature:Talk(TINHEAD_SAY_DEATH)
    -- C++ JustDied arm order: Talk, then SummonCroneIfReady.
    summonCroneIfReady(creature)
end


-- C++ JustReachedHome() -> me->DespawnOrUnsummon() (bosses_opera.cpp);
-- event 24 = CreatureEventOnReachHome (runtime.go:207), Despawn bridged
-- (lua_creature_events.go:214).
local function tinheadReachHome(event, creature)
    tinheadLeaveCombat(event, creature)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_TINHEAD, 1, tinheadEnterCombat)
RegisterCreatureEvent(ENTRY_TINHEAD, 2, tinheadLeaveCombat)
RegisterCreatureEvent(ENTRY_TINHEAD, 3, tinheadTargetDied)
RegisterCreatureEvent(ENTRY_TINHEAD, 4, tinheadDied)
RegisterCreatureEvent(ENTRY_TINHEAD, 23, tinheadReset)
RegisterCreatureEvent(ENTRY_TINHEAD, 24, tinheadReachHome)

-- Roar (boss_roar, C++ ScriptName for 17546): mangle 31041 on the victim
-- every 5-8s; shred 31042 on the victim every 10-15s; frightened scream
-- 31013 on the victim every 20-30s.
local ROAR_SAY_AGGRO = 0
local ROAR_SAY_DEATH = 1
local ROAR_SAY_SLAY = 2

local SPELL_MANGLE = 31041
local SPELL_SHRED = 31042
local SPELL_FRIGHTENED_SCREAM = 31013

local function roarMangle(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_MANGLE)
    end
    schedule(guid, "mangle", math.random(5000, 8000), function()
        roarMangle(creature, guid)
    end)
end

local function roarShred(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_SHRED)
    end
    schedule(guid, "shred", math.random(10000, 15000), function()
        roarShred(creature, guid)
    end)
end

local function roarScream(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_FRIGHTENED_SCREAM)
    end
    schedule(guid, "scream", math.random(20000, 30000), function()
        roarScream(creature, guid)
    end)
end

local function roarEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(ROAR_SAY_AGGRO)
    schedule(guid, "mangle", 5000, function()
        roarMangle(creature, guid)
    end)
    schedule(guid, "shred", 10000, function()
        roarShred(creature, guid)
    end)
    schedule(guid, "scream", 15000, function()
        roarScream(creature, guid)
    end)
end

local function roarLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_roarAI::UpdateAI AggroTimer (bosses_opera.cpp:628-632):
-- 20s after spawn/evade the template-set NON_ATTACKABLE flag is
-- cleared (AggroTimer=20000 in Initialize, :567; no SetFlag in file).
local function roarReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    scheduleIntroReset(guid, creature, 20000)
end

local function roarTargetDied(event, creature, victim)
    -- C++ KilledUnit Talks unconditionally (no TYPEID_PLAYER gate).
    if victim ~= nil then
        creature:Talk(ROAR_SAY_SLAY)
    end
end

local function roarDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(ROAR_SAY_DEATH)
    -- C++ JustDied arm order: Talk, then SummonCroneIfReady.
    summonCroneIfReady(creature)
end


-- C++ JustReachedHome() -> me->DespawnOrUnsummon() (bosses_opera.cpp);
-- event 24 = CreatureEventOnReachHome (runtime.go:207), Despawn bridged
-- (lua_creature_events.go:214).
local function roarReachHome(event, creature)
    roarLeaveCombat(event, creature)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_ROAR, 1, roarEnterCombat)
RegisterCreatureEvent(ENTRY_ROAR, 2, roarLeaveCombat)
RegisterCreatureEvent(ENTRY_ROAR, 3, roarTargetDied)
RegisterCreatureEvent(ENTRY_ROAR, 4, roarDied)
RegisterCreatureEvent(ENTRY_ROAR, 23, roarReset)
RegisterCreatureEvent(ENTRY_ROAR, 24, roarReachHome)

-- The Crone (boss_crone, C++ ScriptName for 18168): chain lightning 32337
-- on the victim every 15s (first 10s). The cyclone summon (30s) never
-- fires — no summon model. The spawn-hack RemoveFlag/SetImmuneToPC arms
-- are PORTED in croneLeaveCombat (ev2+ev23 = C++ Reset path).
local CRONE_SAY_AGGRO = 0
local CRONE_SAY_DEATH = 1
local CRONE_SAY_SLAY = 2

local SPELL_CHAIN_LIGHTNING = 32337

local function croneChainLightning(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CHAIN_LIGHTNING)
    end
    schedule(guid, "chain", 15000, function()
        croneChainLightning(creature, guid)
    end)
end

local function croneEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(CRONE_SAY_AGGRO)
    schedule(guid, "chain", 10000, function()
        croneChainLightning(creature, guid)
    end)
end

local function croneLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ boss_crone::Initialize (bosses_opera.cpp:686-687): the spawn-hack
    -- RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE) +
    -- SetImmuneToPC(false) (= RemoveFlag(59, 0x100), Unit.cpp:8440-8449)
    -- runs on every Reset; ev23 fires on initial spawn, evade-reset and
    -- respawn (lua_creature_events.go:1560, creaturemotion.go:719).
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, 0x100)
end

local function croneTargetDied(event, creature, victim)
    -- C++ KilledUnit Talks unconditionally (no TYPEID_PLAYER gate).
    if victim ~= nil then
        creature:Talk(CRONE_SAY_SLAY)
    end
end

local function croneDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(CRONE_SAY_DEATH)
end


-- C++ JustReachedHome() -> me->DespawnOrUnsummon() (bosses_opera.cpp);
-- event 24 = CreatureEventOnReachHome (runtime.go:207), Despawn bridged
-- (lua_creature_events.go:214).
local function croneReachHome(event, creature)
    croneLeaveCombat(event, creature)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_CRONE, 1, croneEnterCombat)
RegisterCreatureEvent(ENTRY_CRONE, 2, croneLeaveCombat)
RegisterCreatureEvent(ENTRY_CRONE, 3, croneTargetDied)
RegisterCreatureEvent(ENTRY_CRONE, 4, croneDied)
RegisterCreatureEvent(ENTRY_CRONE, 23, croneLeaveCombat)
RegisterCreatureEvent(ENTRY_CRONE, 24, croneReachHome)

-- The Big Bad Wolf (boss_bigbadwolf, C++ ScriptName for 17521): chase
-- every 30s — Talk(SAY_WOLF_HOOD = 2) + triggered Little Red Riding Hood
-- 30768 on a random alive player within 100 yd, then a 20s chase during
-- which fear/swipe are frozen, then 40s calm. The chase threat juggling
-- is ported (bosses_opera.cpp:925-948): GetThreat store + -100% cut +
-- AddThreat 1000000 on chase start, stored HoodGUID/TempThreat restored
-- (-100% gate + AddThreat) on chase end via the GetPlayersInWorld GUID
-- scan (shade_of_aran precedent; nil when the Hood target left = the
-- C++ GetUnit nil arm).
-- Terrifying howl 30752 on the victim every 25-35s; wide swipe 30761 on
-- the victim every 25-30s (first 5s); neither ticks while chasing.
-- The death sound (9275) is ported via PlayDirectSound in wolfDied;
-- the SetBossState(DONE) arm has no instance-script model.
local WOLF_SAY_AGGRO = 0
local WOLF_SAY_SLAY = 1
local WOLF_SAY_HOOD = 2

local SPELL_LITTLE_RED_RIDING_HOOD = 30768
local SPELL_TERRIFYING_HOWL = 30752
local SPELL_WIDE_SWIPE = 30761

local wolfState = {}

-- Forward declarations: wolfArmFear/wolfArmSwipe closures call these
-- (postmaster_malown precedent — a plain local-def-after-use would bind
-- a nil global).
local wolfFear, wolfSwipe

-- C++ freezes FearTimer/SwipeTimer with their remainders while the wolf
-- chases (bosses_opera.cpp:952 "if (IsChasing) return;"). CreateLuaEvent
-- has no pause, so the fire-times are banked in the state (os.time
-- seconds, malchezaar clamp precedent) and the timers are cancelled at
-- chase start, then re-armed with the remainders at chase end.
local function wolfArmFear(creature, guid, delayMs)
    local st = wolfState[guid]
    if st ~= nil then
        st.fearAt = os.time() + delayMs / 1000
    end
    schedule(guid, "fear", delayMs, function()
        wolfFear(creature, guid)
    end)
end

local function wolfArmSwipe(creature, guid, delayMs)
    local st = wolfState[guid]
    if st ~= nil then
        st.swipeAt = os.time() + delayMs / 1000
    end
    schedule(guid, "swipe", delayMs, function()
        wolfSwipe(creature, guid)
    end)
end

wolfFear = function(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_TERRIFYING_HOWL)
    end
    wolfArmFear(creature, guid, math.random(25000, 35000))
end

wolfSwipe = function(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WIDE_SWIPE)
    end
    wolfArmSwipe(creature, guid, math.random(25000, 30000))
end

local function wolfChase(creature, guid)
    local st = wolfState[guid]
    if st == nil then
        st = { chasing = false }
        wolfState[guid] = st
    end
    if not st.chasing then
        local target = randomPlayerInRange(creature, 100)
        if target then
            creature:Talk(WOLF_SAY_HOOD)
            creature:CastSpell(target, SPELL_LITTLE_RED_RIDING_HOOD, true)
            -- C++ arm order: store TempThreat, -100% cut gated on it,
            -- HoodGUID store, AddThreat 1000000.
            st.tempThreat = creature:GetThreat(target)
            if st.tempThreat ~= 0 then
                creature:ModifyThreatByPercent(target, -100)
            end
            st.hoodGUID = target:GetGUID()
            creature:AddThreat(target, 1000000)
            st.chasing = true
            -- Freeze fear/swipe with remainders (C++-exact).
            local now = os.time()
            st.fearRemain = st.fearAt ~= nil and math.max(st.fearAt - now, 0) or 0
            st.swipeRemain = st.swipeAt ~= nil and math.max(st.swipeAt - now, 0) or 0
            cancelKeys(guid, { "fear", "swipe" })
        end
        -- C++ leaves ChaseTimer armed, so a null target retries on the
        -- next tick; only a successful chase starts the 20s chase timer.
        schedule(guid, "chase", target and 20000 or 1000, function()
            wolfChase(creature, guid)
        end)
    else
        st.chasing = false
        -- C++ chase-end order: HoodGUID lookup, clear, -100% gate,
        -- AddThreat(TempThreat), TempThreat = 0.
        local hood = nil
        if st.hoodGUID ~= nil then
            for _, p in ipairs(GetPlayersInWorld()) do
                if p:GetGUID() == st.hoodGUID and p:GetMapId() == creature:GetMapId()
                        and p:GetInstanceId() == creature:GetInstanceId() then
                    hood = p
                    break
                end
            end
        end
        if hood ~= nil then
            st.hoodGUID = nil
            if creature:GetThreat(hood) ~= 0 then
                creature:ModifyThreatByPercent(hood, -100)
            end
            creature:AddThreat(hood, st.tempThreat or 0)
            st.tempThreat = 0
        end
        -- Restore the banked remainders; CreateLuaEvent floors at 1ms
        -- (nightbane precedent).
        wolfArmFear(creature, guid, math.max(st.fearRemain * 1000, 1))
        wolfArmSwipe(creature, guid, math.max(st.swipeRemain * 1000, 1))
        st.fearRemain, st.swipeRemain = nil, nil
        schedule(guid, "chase", 40000, function()
            wolfChase(creature, guid)
        end)
    end
end

local function wolfEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    wolfState[guid] = { chasing = false }
    creature:Talk(WOLF_SAY_AGGRO)
    schedule(guid, "chase", 30000, function()
        wolfChase(creature, guid)
    end)
    wolfArmFear(creature, guid, math.random(25000, 35000))
    wolfArmSwipe(creature, guid, 5000)
end

local function wolfLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    wolfState[guid] = nil
end

local function wolfTargetDied(event, creature, victim)
    -- C++ KilledUnit Talks unconditionally (no TYPEID_PLAYER gate).
    if victim ~= nil then
        creature:Talk(WOLF_SAY_SLAY)
    end
end

-- C++ boss_bigbadwolf::JustDied: DoPlaySoundToSet(me, SOUND_WOLF_DEATH
-- 9275) = creature:PlayDirectSound (SMSG_PLAY_SOUND); the
-- SetBossState(DONE) arm has no instance-script model. Cleanup.
local function wolfDied(event, creature, killer)
    local guid = creature:GetGUID()
    creature:PlayDirectSound(9275)
    cancelTimers(guid)
    wolfState[guid] = nil
end


-- C++ JustReachedHome() -> me->DespawnOrUnsummon() (bosses_opera.cpp);
-- event 24 = CreatureEventOnReachHome (runtime.go:207), Despawn bridged
-- (lua_creature_events.go:214).
local function wolfReachHome(event, creature)
    wolfLeaveCombat(event, creature)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_WOLF, 1, wolfEnterCombat)
RegisterCreatureEvent(ENTRY_WOLF, 2, wolfLeaveCombat)
RegisterCreatureEvent(ENTRY_WOLF, 3, wolfTargetDied)
RegisterCreatureEvent(ENTRY_WOLF, 4, wolfDied)
RegisterCreatureEvent(ENTRY_WOLF, 23, wolfLeaveCombat)
RegisterCreatureEvent(ENTRY_WOLF, 24, wolfReachHome)

-- Grandmother (npc_grandmother, C++ ScriptName for 17603):
-- OnGossipSelect (bosses_opera.cpp:824-834) gates on menu 7443 option 0.
-- The creature select hook now carries the trailing listID arg
-- (gossip.go), so listID == 0 IS the C++ option gate (sender == 0
-- retained; DB items carry sender 0). C++ arm order:
-- CloseGossipMenuFor, summon the
-- Big Bad Wolf (17521) at my position,
-- TEMPSUMMON_TIMED_OR_DEAD_DESPAWN (0), 2h = 7200000ms, then
-- self-despawn. C++ pBigBadWolf->AI()->AttackStart(player) is PORTED
-- below via EngageWithTarget (the boss_bigbadwolf AI above is
-- registered, so the wolf fights once engaged). C++ returns false (AI/native
-- select arms proceed — they do nothing here).
local function grandmotherGossipSelect(event, player, creature, sender, action, code, listID)
    if sender ~= 0 or listID ~= 0 then
        return false
    end
    player:GossipClearMenu()
    player:GossipComplete()
    local wolf = creature:SummonCreature(ENTRY_WOLF, creature:GetX(), creature:GetY(),
        creature:GetZ(), creature:GetO(), 0, 7200000)
    if wolf then
        wolf:EngageWithTarget(player)
    end
    creature:Despawn()
    return false
end

RegisterCreatureGossipEvent(ENTRY_GRANDMOTHER, 2, grandmotherGossipSelect)

-- Romulo and Julianne (boss_julianne / boss_romulo, C++ ScriptNames for
-- 17534 / 17533): the three-phase fake-death dance.
-- Phase JULIANNE: Julianne fights alone — blinding passion 30890 on a
-- random alive player within 100 yd every 30-45s, devotion 30887 on self
-- every 15-45s, powerful attraction 30889 on a random player (any range,
-- alive or dead — C++ SelectTarget(Random, 0) has no distance/alive
-- filter, unlike blinding passion's (Random, 0, 100, true)) every 5-30s, eternal affection 30878 every 45-60s (C++
-- casts it on Romulo half the time once he is up and alive, else on
-- herself — ported C++-exact). Lethal damage in this phase is absorbed: the
-- combat arms freeze at the lethal blow (C++ IsFakingDeath), DoCast(self,
-- drink-poison 30907, 2-arg non-triggered) then SpellHit fires
-- Talk(SAY_JULIANNE_DEATH01 = 2), and 2.5s later she fakes death and flips
-- to phase ROMULO; 10s after that Romulo is summoned (summon bridge,
-- 2026-10-22 — his DoZoneInCombat aggro leg (bosses_opera.cpp:1411,
-- on the summon, not self) IS PORTED: re-evaluated 2026-10-23 —
-- summon:DoZoneInCombat() with the summon object in hand (the
-- "documented gap" note is stale).
-- Phase ROMULO: Romulo fights — backward lunge 30815 on a random alive
-- player within 100 yd every 15-30s (C++ picks the second-threat target
-- behind him; no threat/facing model), daring 30841 on self every 20-40s,
-- deadly swathe 30817 on a random alive player within 100 yd every
-- 15-25s, poison thrust 30822 on the victim every 10-20s. Lethal damage:
-- Talk(SAY_ROMULO_DEATH = 1), he fakes death, and 10s later Julianne
-- resurrects herself (Talk-free, res visual 24171) and 1s after that
-- raises Romulo with Talk(SAY_JULIANNE_RESURRECT = 4) — phase BOTH.
-- Phase BOTH: a lethal blow on either fakes them out and the partner
-- raises them 10s later (Julianne: Talk(4); Romulo:
-- Talk(SAY_ROMULO_RESURRECT = 3), both with the 24171 visual); a lethal
-- blow on one while the partner is already faking kills them for real —
-- the partner's forced real death has no cross-creature bridge, so the
-- faking partner stays down (documented). The pre-combat EntryYell/
-- AggroYell timers and the NON_ATTACKABLE/faction arms have no bridge.
-- Julianne's SpellHit(DRINK_POISON) arm is folded into the event-9 hook.
local JULIANNE_SAY_AGGRO = 0
local JULIANNE_SAY_DEATH01 = 2
local JULIANNE_SAY_DEATH02 = 3
local JULIANNE_SAY_RESURRECT = 4
local JULIANNE_SAY_SLAY = 5

local ROMULO_SAY_AGGRO = 0
local ROMULO_SAY_DEATH = 1
local ROMULO_SAY_RESURRECT = 3
local ROMULO_SAY_SLAY = 4

local SPELL_BLINDING_PASSION = 30890
local SPELL_DEVOTION = 30887
local SPELL_ETERNAL_AFFECTION = 30878
local SPELL_POWERFUL_ATTRACTION = 30889
local SPELL_DRINK_POISON = 30907
local SPELL_BACKWARD_LUNGE = 30815
local SPELL_DARING = 30841
local SPELL_DEADLY_SWATHE = 30817
local SPELL_POISON_THRUST = 30822
local SPELL_RES_VISUAL = 24171

local PHASE_JULIANNE = 0
local PHASE_ROMULO = 1
local PHASE_BOTH = 2

-- bosses_opera.cpp:1006-1007 (ROMULO_X/Y); SharedDefines.h:227
-- (FACTION_MONSTER_2).
local ROMULO_X = -10900
local ROMULO_Y = -1758
local FACTION_MONSTER_2 = 16

local JULIANNE_COMBAT_KEYS = { "blinding", "devotion", "attraction", "affection" }
local ROMULO_COMBAT_KEYS = { "lunge", "daring", "swathe", "thrust" }

local raj = {
    phase = PHASE_JULIANNE,
    julianne = nil,
    julianneGuid = nil,
    romulo = nil,
    romuloGuid = nil,
    julianneFaking = false,
    romuloFaking = false,
    romuloDead = false,
    julianneDead = false,
}

-- C++ UpdateAI early-returns on IsFakingDeath (bosses_opera.cpp ~1315/1430)
-- without touching the combat timers, so they resume with their remainders
-- when the fake ends. CreateLuaEvent has no pause, so the wolf's
-- deadline-banking pattern is reused: rajSchedule records each combat arm's
-- fire-time per GUID+key (os.time seconds), rajBankRemainders freezes the
-- remainders at pretend-die time, and the ReArmCombat variants re-arm with
-- the banked remainder (0 -> 1ms: CreateLuaEvent floors at 1ms, so a zero
-- remainder fires next tick, the C++ analogue) or the combat-initial delay
-- when nothing was banked (the partner was never in combat).
local rajDeadlines = {}

local function rajSchedule(guid, key, delayMs, fn)
    local dl = rajDeadlines[guid]
    if dl == nil then
        dl = {}
        rajDeadlines[guid] = dl
    end
    dl[key] = os.time() + delayMs / 1000
    schedule(guid, key, delayMs, fn)
end

local function rajBankRemainders(guid, keys)
    local dl = rajDeadlines[guid] or {}
    local now = os.time()
    local frozen = {}
    for _, key in ipairs(keys) do
        frozen[key] = dl[key] ~= nil and math.max(dl[key] - now, 0) or nil
    end
    return frozen
end

local julianneArmCombat
local romuloArmCombat
local julianneReArmCombat
local romuloReArmCombat
local julianneResurrectSelf
local julianneResurrectRomulo
local romuloResurrectJulianne

local function julianneBlindingPassion(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_BLINDING_PASSION)
    end
    rajSchedule(guid, "blinding", math.random(30000, 45000), function()
        julianneBlindingPassion(creature, guid)
    end)
end

local function julianneDevotion(creature, guid)
    creature:CastSpell(creature, SPELL_DEVOTION)
    rajSchedule(guid, "devotion", math.random(15000, 45000), function()
        julianneDevotion(creature, guid)
    end)
end

local function julianneAttraction(creature, guid)
    -- C++: DoCast(SelectTarget(SelectTargetMethod::Random, 0) (:1469),
    -- SPELL_POWERFUL_ATTRACTION) — no 100yd limit; C++
    -- SelectTargetList skips offline refs before the pick (UnitAI.h),
    -- unlike Blinding Passion (Random, 0, 100, true); playerOnly=FALSE
    -- (UnitAI.h:170) so the pool is the threat list (pets/totems
    -- eligible), lowered via GetAITargets (lua_creature_events.go:345 —
    -- the midnight precedent).
    local target = randomThreatTarget(creature)
    if target then
        creature:CastSpell(target, SPELL_POWERFUL_ATTRACTION)
    end
    rajSchedule(guid, "attraction", math.random(5000, 30000), function()
        julianneAttraction(creature, guid)
    end)
end

local function julianneAffection(creature, guid)
    -- C++ EVENT_ETERNAL_AFFECTION (bosses_opera.cpp:1474-1482):
    -- urand(0,1) and SummonedRomulo with Romulo alive and not RomuloDead
    -- -> cast on Romulo; otherwise self.
    local romulo = raj.romulo
    if math.random(0, 1) == 1 and raj.romuloSummoned and romulo ~= nil
            and not raj.romuloDead and not romulo:IsDead() then
        creature:CastSpell(romulo, SPELL_ETERNAL_AFFECTION)
    else
        creature:CastSpell(creature, SPELL_ETERNAL_AFFECTION)
    end
    rajSchedule(guid, "affection", math.random(45000, 60000), function()
        julianneAffection(creature, guid)
    end)
end

julianneArmCombat = function(creature, guid)
    rajSchedule(guid, "blinding", 30000, function()
        julianneBlindingPassion(creature, guid)
    end)
    rajSchedule(guid, "devotion", 15000, function()
        julianneDevotion(creature, guid)
    end)
    rajSchedule(guid, "attraction", 5000, function()
        julianneAttraction(creature, guid)
    end)
    rajSchedule(guid, "affection", 25000, function()
        julianneAffection(creature, guid)
    end)
end

julianneReArmCombat = function(creature, guid)
    local frozen = raj.julianneFrozen
    raj.julianneFrozen = nil
    local function arm(key, initialMs, armFn)
        local remain = frozen and frozen[key] or nil
        local delayMs = remain ~= nil and math.max(remain * 1000, 1) or initialMs
        rajSchedule(guid, key, delayMs, function()
            armFn(creature, guid)
        end)
    end
    arm("blinding", 30000, julianneBlindingPassion)
    arm("devotion", 15000, julianneDevotion)
    arm("attraction", 5000, julianneAttraction)
    arm("affection", 25000, julianneAffection)
end

local function juliannePretendDie(creature, guid)
    creature:RemoveAllAuras()
    -- Bank the remainders BEFORE the cancel: C++ froze the timers untouched.
    raj.julianneFrozen = rajBankRemainders(guid, JULIANNE_COMBAT_KEYS)
    cancelKeys(guid, JULIANNE_COMBAT_KEYS)
end

local function julianneEnterCombat(event, creature, target)
    -- NO SetMeleeAttack: boss_julianneAI::UpdateAI (bosses_opera.cpp:1368, out-of-line
    -- definition — the :1161 line is a body-less declaration) ENDS with
    -- DoMeleeAttackIfReady(); julianne swings in C++. A 2026-10-22 port added
    -- SetMeleeAttack(false) citing the declaration; removed — it suppressed a real
    -- C++ swing (romulo's UpdateAI also swings; neither gets the no-melee flag).
    local guid = creature:GetGUID()
    local guid = creature:GetGUID()
    cancelTimers(guid)
    raj.julianne = creature
    raj.julianneGuid = guid
    julianneArmCombat(creature, guid)
end

local function julianneLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    rajDeadlines[guid] = nil
    raj.julianneFrozen = nil
    if raj.julianneGuid == guid then
        -- C++ Reset resurrects a faking Julianne (bosses_opera.cpp:1104-1110):
        -- Resurrect(me) ahead of clearing the faking flag.
        if raj.julianneFaking then
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
            creature:SetHealth(creature:GetMaxHealth())
            creature:SetStandState(0)
            creature:CastSpell(creature, SPELL_RES_VISUAL, true)
        end
        raj.julianneFaking = false
        raj.romuloDead = false
        raj.romuloSummoned = false
        raj.phase = PHASE_JULIANNE
    end
end

local function julianneTargetDied(event, creature, victim)
    -- C++ KilledUnit Talks unconditionally (no TYPEID_PLAYER gate).
    if victim ~= nil then
        creature:Talk(JULIANNE_SAY_SLAY)
    end
end

local function julianneDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    rajDeadlines[guid] = nil
    raj.julianneFrozen = nil
    creature:Talk(JULIANNE_SAY_DEATH02)
    if raj.julianneGuid == guid then
        raj.julianneFaking = false
        raj.romuloSummoned = false
        raj.phase = PHASE_JULIANNE
    end
end

-- C++ boss_julianneAI::DamageTaken. The event-9 hook fires pre-application
-- and the second return rewrites the damage (consumed by
-- fireCreatureDamageTaken).
local function julianneDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    -- C++ DamageTaken checks damage < GetHealth() FIRST: non-lethal hits
    -- pass through even while faking; only lethal blows take the branches.
    if damage < creature:GetHealth() then
        return false, damage
    end
    if raj.phase == PHASE_JULIANNE then
        -- A lethal blow already drinking returns early with damage nulled
        -- (C++: damage = 0 then the IsFakingDeath return).
        if raj.julianneFaking then
            return false, 0
        end
        raj.julianneFaking = true
        -- C++ sets IsFakingDeath immediately at the lethal blow, so the
        -- combat arms freeze via UpdateAI's early return from that moment;
        -- the 2.5s DrinkPoisonTimer only handles the PretendToDie visual
        -- and the phase flip (bosses_opera.cpp:1140-1146, 1488+).
        juliannePretendDie(creature, guid)
        creature:Talk(JULIANNE_SAY_DEATH01)
        creature:CastSpell(creature, SPELL_DRINK_POISON)
        schedule(guid, "drinkpoison", 2500, function()
            raj.phase = PHASE_ROMULO
            schedule(guid, "summonromulo", 10000, function()
                -- C++ boss_julianneAI::UpdateAI (bosses_opera.cpp:1404-1415):
                -- Phase == PHASE_ROMULO and not SummonedRomulo: summon
                -- Romulo (17533) at (ROMULO_X, ROMULO_Y, my z), o = 0 ->
                -- my orientation (C++ zero-o fallback, Object.cpp:2061),
                -- TEMPSUMMON_TIMED_OR_DEAD_DESPAWN (0), 2h = 7200000ms.
                -- C++ arm order: RomuloGUID latch (raj.romulo/romuloGuid),
                -- ENSURE_AI JulianneGUID + Phase = PHASE_ROMULO (the
                -- shared raj table — single-instance convention),
                -- DoZoneInCombat(pRomulo) :1411 PORTED below (bridge live
                -- 2026-10-23 — receiver-agnostic creature method; the summon
                -- object is in hand, C++ order kept: before SetFaction),
                -- SetFaction(FACTION_MONSTER_2) (bridged, ported).
                local summon = creature:SummonCreature(ENTRY_ROMULO,
                    ROMULO_X, ROMULO_Y, creature:GetZ(), creature:GetO(),
                    0, 7200000)
                if summon ~= nil then
                    summon:DoZoneInCombat()
                    summon:SetFaction(FACTION_MONSTER_2)
                    raj.romulo = summon
                    raj.romuloGuid = summon:GetGUID()
                end
                raj.romuloSummoned = true
                raj.phase = PHASE_ROMULO
            end)
        end)
        return false, 0
    end
    if raj.phase == PHASE_BOTH then
        if raj.romuloDead then
            -- C++ boss_julianneAI::DamageTaken (bosses_opera.cpp:1521-1533)
            -- really kills Romulo here: RemoveFlag(59, NOT_SELECTABLE)
            -- (vacuous — the Lua fake model never sets it),
            -- MotionMaster Clear + setDeathState(JUST_DIED)
            -- (no bridges — he stays faking, documented),
            -- CombatStop(true) (PORTED — clears his combat state at
            -- the C++ position, ahead of the LOOTABLE flag),
            -- SetUInt32Value(79, LOOTABLE) (PORTED — his faking body
            -- becomes lootable like the C++ corpse). His pending
            -- resjulianne timer dies with him in C++; cancel it so he
            -- cannot resurrect the really-dead Julianne. Julianne dies.
            if raj.romulo ~= nil then
                raj.romulo:CombatStop(true)
                raj.romulo:SetFlag(79, 0x0001) -- UNIT_DYNFLAG_LOOTABLE
            end
            if raj.romuloGuid ~= nil then
                cancelKeys(raj.romuloGuid, {"resjulianne"})
            end
            return false, damage
        end
        raj.julianneFaking = true
        raj.julianneDead = true
        juliannePretendDie(creature, guid)
        if raj.romuloGuid ~= nil then
            schedule(raj.romuloGuid, "resjulianne", 10000, function()
                romuloResurrectJulianne()
            end)
        end
        return false, 0
    end
    return false, 0
end

julianneResurrectSelf = function()
    local jguid = raj.julianneGuid
    if jguid == nil or not raj.julianneFaking then
        return
    end
    if raj.julianne ~= nil then
        -- C++ Resurrect (bosses_opera.cpp:1029-1034): RemoveFlag, full
        -- health, stand, then the res visual, in C++ order.
        raj.julianne:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        raj.julianne:SetHealth(raj.julianne:GetMaxHealth())
        raj.julianne:SetStandState(0)
        raj.julianne:CastSpell(raj.julianne, SPELL_RES_VISUAL, true)
    end
    raj.julianneFaking = false
    raj.phase = PHASE_BOTH
    if raj.julianne ~= nil then
        julianneReArmCombat(raj.julianne, jguid)
        -- C++ Resurrect(me) + self-resurrect AttackStart (bosses_opera.cpp:1029-1040,
        -- :1427-1428): re-engage her own victim if she still has one (MoveChase half out).
        local victim = raj.julianne:GetVictim()
        if victim ~= nil then
            raj.julianne:EngageWithTarget(victim)
        end
    end
    schedule(jguid, "resromulo", 1000, function()
        julianneResurrectRomulo()
    end)
end

julianneResurrectRomulo = function()
    local rguid = raj.romuloGuid
    if rguid == nil or not raj.romuloDead then
        return
    end
    -- C++: the scheduler's ResurrectTimer only ticks while she is not
    -- faking (boss_julianneAI::UpdateAI early-returns on IsFakingDeath,
    -- bosses_opera.cpp:1434); a faking scheduler's timer is frozen, so
    -- the wall-clock Lua timer must not fire it.
    if raj.julianneFaking then
        return
    end
    if raj.julianne ~= nil then
        raj.julianne:Talk(JULIANNE_SAY_RESURRECT)
    end
    if raj.romulo ~= nil then
        -- C++ Resurrect (bosses_opera.cpp:1029-1034): RemoveFlag, full
        -- health, stand, then the res visual, in C++ order.
        raj.romulo:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        raj.romulo:SetHealth(raj.romulo:GetMaxHealth())
        raj.romulo:SetStandState(0)
        raj.romulo:CastSpell(raj.romulo, SPELL_RES_VISUAL, true)
    end
    raj.romuloFaking = false
    raj.romuloDead = false
    if raj.romulo ~= nil then
        romuloReArmCombat(raj.romulo, rguid)
        -- C++ Resurrect(target) AttackStart (bosses_opera.cpp:1035-1039): re-engage
        -- his own victim if he still has one (MoveChase half out).
        local victim = raj.romulo:GetVictim()
        if victim ~= nil then
            raj.romulo:EngageWithTarget(victim)
        end
    end
end

romuloResurrectJulianne = function()
    local jguid = raj.julianneGuid
    if jguid == nil or not raj.julianneDead then
        return
    end
    -- C++: the scheduler's ResurrectTimer only ticks while he is not
    -- faking (boss_romuloAI::UpdateAI early-returns on IsFakingDeath,
    -- bosses_opera.cpp:1315); a faking scheduler's timer is frozen.
    if raj.romuloFaking then
        return
    end
    if raj.romulo ~= nil then
        raj.romulo:Talk(ROMULO_SAY_RESURRECT)
    end
    if raj.julianne ~= nil then
        -- C++ Resurrect (bosses_opera.cpp:1029-1034): RemoveFlag, full
        -- health, stand, then the res visual, in C++ order.
        raj.julianne:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        raj.julianne:SetHealth(raj.julianne:GetMaxHealth())
        raj.julianne:SetStandState(0)
        raj.julianne:CastSpell(raj.julianne, SPELL_RES_VISUAL, true)
    end
    raj.julianneFaking = false
    raj.julianneDead = false
    if raj.julianne ~= nil then
        julianneReArmCombat(raj.julianne, jguid)
        -- C++ Resurrect(target) AttackStart (bosses_opera.cpp:1035-1039): re-engage
        -- her own victim if she still has one (MoveChase half out).
        local victim = raj.julianne:GetVictim()
        if victim ~= nil then
            raj.julianne:EngageWithTarget(victim)
        end
    end
end


-- C++ JustReachedHome() -> me->DespawnOrUnsummon() (bosses_opera.cpp);
-- event 24 = CreatureEventOnReachHome (runtime.go:207), Despawn bridged
-- (lua_creature_events.go:214).
local function julianneReachHome(event, creature)
    julianneLeaveCombat(event, creature)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_JULIANNE, 1, julianneEnterCombat)
RegisterCreatureEvent(ENTRY_JULIANNE, 2, julianneLeaveCombat)
RegisterCreatureEvent(ENTRY_JULIANNE, 3, julianneTargetDied)
RegisterCreatureEvent(ENTRY_JULIANNE, 4, julianneDied)
RegisterCreatureEvent(ENTRY_JULIANNE, 9, julianneDamageTaken)
RegisterCreatureEvent(ENTRY_JULIANNE, 23, julianneLeaveCombat)
RegisterCreatureEvent(ENTRY_JULIANNE, 24, julianneReachHome)

-- C++ EVENT_BACKWARD_LUNGE (:1335-1343): SelectTarget(Random, 1, 100, true)
-- (offset 1 = victim excluded, akilzon idiom) + !me->HasInArc(M_PI)
-- (behind-target gate via the inArc twin above). The timer resets
-- urand(15000,30000) ONLY on a successful cast; a failed gate or empty
-- pick leaves the timer expired for a next-tick retry
-- (yazzai-Polymorph precedent).
local function romuloLunge(creature, guid)
    -- C++ SelectTarget(Random, 1, 100, true) (:1336): UnitAI.h builds
    -- the list victim-first, so offset 1 pops the victim BEFORE the
    -- player/alive/range filters (akilzon idiom); GetAITargets is
    -- threat-sorted, so a positional i > 1 drop would cut the threat
    -- head instead under fixate/taunt.
    local pool = {}
    local victim = creature:GetVictim()
    local victimGuid = nil
    if victim ~= nil then
        victimGuid = victim:GetGUID()
    end
    local targets = creature:GetAITargets()
    local reach = creature:GetCombatReach()
    local aliveSeen = 0
    for _, unit in ipairs(targets) do
        local skip = false
        if victimGuid ~= nil then
            skip = unit:GetGUID() == victimGuid
        elseif not unit:IsDead() then
            aliveSeen = aliveSeen + 1
            skip = aliveSeen == 1
        end
        if not skip
                and unit:GetObjectType() == "Player" and not unit:IsDead()
                and creature:GetDistance(unit) - reach - 1.5 < 100 then -- IsWithinCombatRange gate (Unit.cpp:583; player reach 1.5, ObjectDefines.h:40)
            pool[#pool + 1] = unit
        end
    end
    if #pool > 0 then
        local target = pool[math.random(#pool)]
        -- C++ !me->HasInArc(M_PI, target) (bosses_opera.cpp:1337):
        -- the atan2 inArc twin was retired 2026-10-24 for the live
        -- creature:IsInBack bridge, twin-exact.
        if creature:IsInBack(target) then
            creature:CastSpell(target, SPELL_BACKWARD_LUNGE)
            rajSchedule(guid, "lunge", math.random(15000, 30000), function()
                romuloLunge(creature, guid)
            end)
            return
        end
    end
    rajSchedule(guid, "lunge", 1000, function()
        romuloLunge(creature, guid)
    end)
end

local function romuloDaring(creature, guid)
    creature:CastSpell(creature, SPELL_DARING)
    rajSchedule(guid, "daring", math.random(20000, 40000), function()
        romuloDaring(creature, guid)
    end)
end

local function romuloSwathe(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_DEADLY_SWATHE)
    end
    rajSchedule(guid, "swathe", math.random(15000, 25000), function()
        romuloSwathe(creature, guid)
    end)
end

local function romuloThrust(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_POISON_THRUST)
    end
    rajSchedule(guid, "thrust", math.random(10000, 20000), function()
        romuloThrust(creature, guid)
    end)
end

romuloArmCombat = function(creature, guid)
    rajSchedule(guid, "lunge", 15000, function()
        romuloLunge(creature, guid)
    end)
    rajSchedule(guid, "daring", 20000, function()
        romuloDaring(creature, guid)
    end)
    rajSchedule(guid, "swathe", 25000, function()
        romuloSwathe(creature, guid)
    end)
    rajSchedule(guid, "thrust", 10000, function()
        romuloThrust(creature, guid)
    end)
end

romuloReArmCombat = function(creature, guid)
    local frozen = raj.romuloFrozen
    raj.romuloFrozen = nil
    local function arm(key, initialMs, armFn)
        local remain = frozen and frozen[key] or nil
        local delayMs = remain ~= nil and math.max(remain * 1000, 1) or initialMs
        rajSchedule(guid, key, delayMs, function()
            armFn(creature, guid)
        end)
    end
    arm("lunge", 15000, romuloLunge)
    arm("daring", 20000, romuloDaring)
    arm("swathe", 25000, romuloSwathe)
    arm("thrust", 10000, romuloThrust)
end

local function romuloPretendDie(creature, guid)
    creature:RemoveAllAuras()
    -- Bank the remainders BEFORE the cancel: C++ froze the timers untouched.
    raj.romuloFrozen = rajBankRemainders(guid, ROMULO_COMBAT_KEYS)
    cancelKeys(guid, ROMULO_COMBAT_KEYS)
end

local function romuloEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    raj.romulo = creature
    raj.romuloGuid = guid
    creature:Talk(ROMULO_SAY_AGGRO)
    romuloArmCombat(creature, guid)
end

local function romuloLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    rajDeadlines[guid] = nil
    raj.romuloFrozen = nil
    if raj.romuloGuid == guid then
        raj.romuloFaking = false
        raj.julianneDead = false
        raj.phase = PHASE_ROMULO
    end
end

local function romuloTargetDied(event, creature, victim)
    -- C++ KilledUnit Talks unconditionally (no TYPEID_PLAYER gate).
    if victim ~= nil then
        creature:Talk(ROMULO_SAY_SLAY)
    end
end

local function romuloDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(ROMULO_SAY_DEATH)
    if raj.romuloGuid == guid then
        raj.romuloFaking = false
        raj.phase = PHASE_ROMULO
    end
end

-- C++ boss_romuloAI::DamageTaken, same event-9 second-return convention.
local function romuloDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    -- C++ checks damage < GetHealth() first: non-lethal hits pass through
    -- even while faking.
    if damage < creature:GetHealth() then
        return false, damage
    end
    if raj.phase == PHASE_ROMULO then
        if raj.romuloFaking then
            return false, 0
        end
        creature:Talk(ROMULO_SAY_DEATH)
        raj.romuloFaking = true
        raj.romuloDead = true
        raj.phase = PHASE_BOTH
        romuloPretendDie(creature, guid)
        if raj.julianneGuid ~= nil then
            schedule(raj.julianneGuid, "resself", 10000, function()
                julianneResurrectSelf()
            end)
        end
        return false, 0
    end
    if raj.phase == PHASE_BOTH then
        if raj.julianneDead then
            -- C++ boss_romuloAI::DamageTaken (bosses_opera.cpp:1252-1262)
            -- really kills Julianne here: RemoveFlag(59, NOT_SELECTABLE)
            -- (vacuous — the Lua fake model never sets it),
            -- MotionMaster Clear + setDeathState(JUST_DIED)
            -- (no bridges — she stays faking, documented),
            -- CombatStop(true) (PORTED — clears her combat state at
            -- the C++ position, ahead of the LOOTABLE flag),
            -- SetUInt32Value(79, LOOTABLE) (PORTED — her faking body
            -- becomes lootable like the C++ corpse, via the
            -- creature:SetFlag(79) dynamic-flag bridge). Her pending
            -- resromulo timer dies with her in C++ (a dead AI ticks
            -- nothing); cancel it so she cannot resurrect the
            -- really-dead Romulo. Romulo dies.
            if raj.julianne ~= nil then
                raj.julianne:CombatStop(true)
                raj.julianne:SetFlag(79, 0x0001) -- UNIT_DYNFLAG_LOOTABLE
            end
            if raj.julianneGuid ~= nil then
                cancelKeys(raj.julianneGuid, {"resromulo"})
            end
            return false, damage
        end
        raj.romuloFaking = true
        raj.romuloDead = true
        romuloPretendDie(creature, guid)
        if raj.julianneGuid ~= nil then
            schedule(raj.julianneGuid, "resromulo", 10000, function()
                julianneResurrectRomulo()
            end)
        end
        return false, 0
    end
    return false, 0
end


-- C++ JustReachedHome() -> me->DespawnOrUnsummon() (bosses_opera.cpp);
-- event 24 = CreatureEventOnReachHome (runtime.go:207), Despawn bridged
-- (lua_creature_events.go:214).
local function romuloReachHome(event, creature)
    romuloLeaveCombat(event, creature)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_ROMULO, 1, romuloEnterCombat)
RegisterCreatureEvent(ENTRY_ROMULO, 2, romuloLeaveCombat)
RegisterCreatureEvent(ENTRY_ROMULO, 3, romuloTargetDied)
RegisterCreatureEvent(ENTRY_ROMULO, 4, romuloDied)
RegisterCreatureEvent(ENTRY_ROMULO, 9, romuloDamageTaken)
RegisterCreatureEvent(ENTRY_ROMULO, 23, romuloLeaveCombat)
RegisterCreatureEvent(ENTRY_ROMULO, 24, romuloReachHome)
