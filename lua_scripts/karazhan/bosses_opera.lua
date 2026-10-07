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
-- Deviations from C++: no summon model — Tito, the Crone
-- (SummonCroneIfReady), the cyclone, Romulo, and the wolf (grandmother
-- gossip) never spawn, so their summon casts are skipped while the
-- non-summon arms stay; npc_tito (17548) is still registered so any
-- spawned Tito fights correctly (aran-elemental convention). No
-- instance-script model — the Oz death counter, DATA_OPERA_PERFORMANCE
-- SetBossState(DONE), and DoZoneInCombat arms are skipped. No threat
-- model — the wolf's chase threat juggling is skipped (chase is Talk +
-- debuff + a fear/swipe freeze) and BackwardLunge picks any random alive
-- player (C++ takes the second-threat target). No UNIT_FLAG model — the
-- Oz AggroTimer RemoveFlag(NON_ATTACKABLE) arms, PretendToDie's
-- NOT_SELECTABLE/SetHealth(0)/stand-state arms, and Resurrect's
-- flag/health/stand-state restores have no bridge; fake death is
-- timer-cancel plus shared flags. No UNIT_STATE_CASTING model — timers
-- fire unconditionally (InterruptNonMeleeSpells skipped). No facing model —
-- BackwardLunge's behind-target (HasInArc) check is skipped. The
-- strawman fire-school SpellHit arm (Burning Straw 31075) is skipped —
-- event 14 carries only the spell id, no school mask. The crone's
-- RemoveFlag/SetImmuneToPC spawn hack has no bridge. npc_cyclone (18412)
-- is not registered: movement-only AI, no combat hooks ever fire.
-- npc_grandmother (17603) is not registered: the gossip select's only
-- effects are the wolf summon (no bearer) and self-despawn — registering
-- it would strand the event (nightbane-urn precedent). The wolf's death
-- sound (9275) has no sound bridge on the motion creature. Julianne's and
-- Romulo's pre-combat EntryYell/AggroYell timers have no Lua-modelable
-- pre-combat tick (their NON_ATTACKABLE/faction arms have no bridge
-- either). Julianne's SpellHit(DRINK_POISON) arm is folded into the
-- event-9 hook — the only caster is her own event-9 arm, so the id-only
-- spell-hit hook adds nothing. PretendToDie's timer freeze is
-- approximated by cancelling combat timers and re-arming them at initial
-- values on resurrect (C++ resumes the frozen timers). The partner's
-- real-death cross-kill (setDeathState JUST_DIED etc.) has no bridge —
-- the survivor's OnDied runs while the faking partner stays down. The
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
local ENTRY_JULIANNE = 17534
local ENTRY_ROMULO = 17533

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

local function alivePlayersInRange(creature, maxDist)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() and creature:GetDistance(p) <= maxDist then
            found[#found + 1] = p
        end
    end
    return found
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0) with no distance or
-- alive filter (Julianne's Powerful Attraction): any player on the map.
local function playersOnMap(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local found = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId then
            found[#found + 1] = p
        end
    end
    return found
end

local function randomPlayerInRange(creature, maxDist)
    local candidates = alivePlayersInRange(creature, maxDist)
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- Dorothee (boss_dorothee, C++ ScriptName for 17535): water bolt 31012 on
-- a random alive player within 100 yd every 5s (first 5s); scream 31013
-- on the victim every 30s (first 15s). The Tito summon (47500ms) never
-- fires — no summon model — so SAY_SUMMON never plays and the TitoDied
-- water-bolt speedup has no trigger.
local DOROTHEE_SAY_DEATH = 0
local DOROTHEE_SAY_AGGRO = 3

local SPELL_WATERBOLT = 31012
local SPELL_SCREAM = 31013

local function dorotheeWaterbolt(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_WATERBOLT)
    end
    schedule(guid, "waterbolt", 5000, function()
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

local function dorotheeEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(DOROTHEE_SAY_AGGRO)
    schedule(guid, "waterbolt", 5000, function()
        dorotheeWaterbolt(creature, guid)
    end)
    schedule(guid, "fear", 15000, function()
        dorotheeFear(creature, guid)
    end)
end

local function dorotheeLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function dorotheeDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(DOROTHEE_SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_DOROTHEE, 1, dorotheeEnterCombat)
RegisterCreatureEvent(ENTRY_DOROTHEE, 2, dorotheeLeaveCombat)
RegisterCreatureEvent(ENTRY_DOROTHEE, 4, dorotheeDied)
RegisterCreatureEvent(ENTRY_DOROTHEE, 23, dorotheeLeaveCombat)

-- Tito (npc_tito, C++ ScriptName for 17548): yipping 31015 on the victim
-- every 10s. Never summoned (no summon model); registered so any spawned
-- Tito fights correctly. The JustDied → Dorothee TitoDied arm needs the
-- cross-AI pointer, no bridge.
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

RegisterCreatureEvent(ENTRY_TITO, 1, titoEnterCombat)
RegisterCreatureEvent(ENTRY_TITO, 2, titoLeaveCombat)
RegisterCreatureEvent(ENTRY_TITO, 4, titoLeaveCombat)
RegisterCreatureEvent(ENTRY_TITO, 23, titoLeaveCombat)

-- Strawman (boss_strawman, C++ ScriptName for 17543): brain bash 31046 on
-- the victim every 15s (first 5s); brain wipe 31069 on a random alive
-- player within 100 yd every 20s (first 7s). The fire-school SpellHit arm
-- (burning straw 31075) has no bridge — event 14 carries only the spell
-- id. The AggroTimer NON_ATTACKABLE arm has no flag bridge.
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

local function strawmanTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(STRAWMAN_SAY_SLAY)
    end
end

local function strawmanDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(STRAWMAN_SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_STRAWMAN, 1, strawmanEnterCombat)
RegisterCreatureEvent(ENTRY_STRAWMAN, 2, strawmanLeaveCombat)
RegisterCreatureEvent(ENTRY_STRAWMAN, 3, strawmanTargetDied)
RegisterCreatureEvent(ENTRY_STRAWMAN, 4, strawmanDied)
RegisterCreatureEvent(ENTRY_STRAWMAN, 23, strawmanLeaveCombat)

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

local function tinheadTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(TINHEAD_SAY_SLAY)
    end
end

local function tinheadDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    tinheadState[guid] = nil
    creature:Talk(TINHEAD_SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_TINHEAD, 1, tinheadEnterCombat)
RegisterCreatureEvent(ENTRY_TINHEAD, 2, tinheadLeaveCombat)
RegisterCreatureEvent(ENTRY_TINHEAD, 3, tinheadTargetDied)
RegisterCreatureEvent(ENTRY_TINHEAD, 4, tinheadDied)
RegisterCreatureEvent(ENTRY_TINHEAD, 23, tinheadLeaveCombat)

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

local function roarTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(ROAR_SAY_SLAY)
    end
end

local function roarDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(ROAR_SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_ROAR, 1, roarEnterCombat)
RegisterCreatureEvent(ENTRY_ROAR, 2, roarLeaveCombat)
RegisterCreatureEvent(ENTRY_ROAR, 3, roarTargetDied)
RegisterCreatureEvent(ENTRY_ROAR, 4, roarDied)
RegisterCreatureEvent(ENTRY_ROAR, 23, roarLeaveCombat)

-- The Crone (boss_crone, C++ ScriptName for 18168): chain lightning 32337
-- on the victim every 15s (first 10s). The cyclone summon (30s) never
-- fires — no summon model. The spawn-hack RemoveFlag/SetImmuneToPC arms
-- have no bridge.
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
end

local function croneTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(CRONE_SAY_SLAY)
    end
end

local function croneDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(CRONE_SAY_DEATH)
end

RegisterCreatureEvent(ENTRY_CRONE, 1, croneEnterCombat)
RegisterCreatureEvent(ENTRY_CRONE, 2, croneLeaveCombat)
RegisterCreatureEvent(ENTRY_CRONE, 3, croneTargetDied)
RegisterCreatureEvent(ENTRY_CRONE, 4, croneDied)
RegisterCreatureEvent(ENTRY_CRONE, 23, croneLeaveCombat)

-- The Big Bad Wolf (boss_bigbadwolf, C++ ScriptName for 17521): chase
-- every 30s — Talk(SAY_WOLF_HOOD = 2) + triggered Little Red Riding Hood
-- 30768 on a random alive player within 100 yd, then a 20s chase during
-- which fear/swipe are frozen (no threat model: the GetThreat/
-- ModifyThreatByPercent/AddThreat juggling is skipped), then 40s calm.
-- Terrifying howl 30752 on the victim every 25-35s; wide swipe 30761 on
-- the victim every 25-30s (first 5s); neither ticks while chasing. The
-- death sound (9275) has no sound bridge; the SetBossState(DONE) arm has
-- no instance-script model.
local WOLF_SAY_AGGRO = 0
local WOLF_SAY_SLAY = 1
local WOLF_SAY_HOOD = 2

local SPELL_LITTLE_RED_RIDING_HOOD = 30768
local SPELL_TERRIFYING_HOWL = 30752
local SPELL_WIDE_SWIPE = 30761

local wolfState = {}

local function wolfFear(creature, guid)
    local st = wolfState[guid]
    if st ~= nil and st.chasing then
        schedule(guid, "fear", 1000, function()
            wolfFear(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_TERRIFYING_HOWL)
    end
    schedule(guid, "fear", math.random(25000, 35000), function()
        wolfFear(creature, guid)
    end)
end

local function wolfSwipe(creature, guid)
    local st = wolfState[guid]
    if st ~= nil and st.chasing then
        schedule(guid, "swipe", 1000, function()
            wolfSwipe(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_WIDE_SWIPE)
    end
    schedule(guid, "swipe", math.random(25000, 30000), function()
        wolfSwipe(creature, guid)
    end)
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
            st.chasing = true
        end
        -- C++ leaves ChaseTimer armed, so a null target retries on the
        -- next tick; only a successful chase starts the 20s chase timer.
        schedule(guid, "chase", target and 20000 or 1000, function()
            wolfChase(creature, guid)
        end)
    else
        st.chasing = false
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
    schedule(guid, "fear", math.random(25000, 35000), function()
        wolfFear(creature, guid)
    end)
    schedule(guid, "swipe", 5000, function()
        wolfSwipe(creature, guid)
    end)
end

local function wolfLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    wolfState[guid] = nil
end

local function wolfTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(WOLF_SAY_SLAY)
    end
end

local function wolfDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    wolfState[guid] = nil
end

RegisterCreatureEvent(ENTRY_WOLF, 1, wolfEnterCombat)
RegisterCreatureEvent(ENTRY_WOLF, 2, wolfLeaveCombat)
RegisterCreatureEvent(ENTRY_WOLF, 3, wolfTargetDied)
RegisterCreatureEvent(ENTRY_WOLF, 4, wolfDied)
RegisterCreatureEvent(ENTRY_WOLF, 23, wolfLeaveCombat)

-- Romulo and Julianne (boss_julianne / boss_romulo, C++ ScriptNames for
-- 17534 / 17533): the three-phase fake-death dance.
-- Phase JULIANNE: Julianne fights alone — blinding passion 30890 on a
-- random alive player within 100 yd every 30-45s, devotion 30887 on self
-- every 15-45s, powerful attraction 30889 on a random player (any range,
-- alive or dead — C++ SelectTarget(Random, 0) has no distance/alive
-- filter, unlike blinding passion's (Random, 0, 100, true)) every 5-30s, eternal affection 30878 on self every 45-60s (C++
-- casts it on Romulo half the time once he is up; he never spawns, so it
-- always lands on Julianne). Lethal damage in this phase is absorbed:
-- Talk(SAY_JULIANNE_DEATH01 = 2), a triggered drink-poison 30907 visual on
-- self, and 2.5s later she fakes death; 10s after that Romulo would be
-- summoned — no summon model, so the encounter stalls here unless a
-- Romulo has entered combat by other means (the link is picked up then).
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

local julianneArmCombat
local romuloArmCombat
local julianneResurrectSelf
local julianneResurrectRomulo
local romuloResurrectJulianne

local function julianneBlindingPassion(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_BLINDING_PASSION)
    end
    schedule(guid, "blinding", math.random(30000, 45000), function()
        julianneBlindingPassion(creature, guid)
    end)
end

local function julianneDevotion(creature, guid)
    creature:CastSpell(creature, SPELL_DEVOTION)
    schedule(guid, "devotion", math.random(15000, 45000), function()
        julianneDevotion(creature, guid)
    end)
end

local function julianneAttraction(creature, guid)
    -- C++: DoCast(SelectTarget(SelectTargetMethod::Random, 0),
    -- SPELL_POWERFUL_ATTRACTION) — no 100yd limit, no alive filter,
    -- unlike Blinding Passion (Random, 0, 100, true).
    local candidates = playersOnMap(creature)
    local target = #candidates > 0 and candidates[math.random(#candidates)] or nil
    if target then
        creature:CastSpell(target, SPELL_POWERFUL_ATTRACTION)
    end
    schedule(guid, "attraction", math.random(5000, 30000), function()
        julianneAttraction(creature, guid)
    end)
end

local function julianneAffection(creature, guid)
    creature:CastSpell(creature, SPELL_ETERNAL_AFFECTION)
    schedule(guid, "affection", math.random(45000, 60000), function()
        julianneAffection(creature, guid)
    end)
end

julianneArmCombat = function(creature, guid)
    schedule(guid, "blinding", 30000, function()
        julianneBlindingPassion(creature, guid)
    end)
    schedule(guid, "devotion", 15000, function()
        julianneDevotion(creature, guid)
    end)
    schedule(guid, "attraction", 5000, function()
        julianneAttraction(creature, guid)
    end)
    schedule(guid, "affection", 25000, function()
        julianneAffection(creature, guid)
    end)
end

local function juliannePretendDie(creature, guid)
    cancelKeys(guid, JULIANNE_COMBAT_KEYS)
end

local function julianneEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    raj.julianne = creature
    raj.julianneGuid = guid
    julianneArmCombat(creature, guid)
end

local function julianneLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    if raj.julianneGuid == guid then
        raj.julianneFaking = false
        raj.romuloDead = false
        raj.phase = PHASE_JULIANNE
    end
end

local function julianneTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(JULIANNE_SAY_SLAY)
    end
end

local function julianneDied(event, creature, killer)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:Talk(JULIANNE_SAY_DEATH02)
    if raj.julianneGuid == guid then
        raj.julianneFaking = false
        raj.phase = PHASE_JULIANNE
    end
end

-- C++ boss_julianneAI::DamageTaken. The event-9 hook fires pre-application
-- and the second return rewrites the damage (consumed by
-- fireCreatureDamageTaken).
local function julianneDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if raj.julianneFaking then
        return false, 0
    end
    if damage < creature:GetHealth() then
        return false, damage
    end
    if raj.phase == PHASE_JULIANNE then
        raj.julianneFaking = true
        creature:Talk(JULIANNE_SAY_DEATH01)
        creature:CastSpell(creature, SPELL_DRINK_POISON, true)
        schedule(guid, "drinkpoison", 2500, function()
            juliannePretendDie(creature, guid)
            raj.phase = PHASE_ROMULO
            schedule(guid, "summonromulo", 10000, function()
                -- No summon model: Romulo never spawns. If one has entered
                -- combat by other means, link it so the dance continues;
                -- otherwise the encounter stalls here (documented gap).
                if raj.romuloGuid ~= nil and not raj.romuloFaking then
                    raj.phase = PHASE_ROMULO
                end
            end)
        end)
        return false, 0
    end
    if raj.phase == PHASE_BOTH then
        if raj.romuloDead then
            -- C++ kills Romulo for real here; no cross-creature kill
            -- bridge, so he stays faking (documented). Julianne dies.
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
        raj.julianne:CastSpell(raj.julianne, SPELL_RES_VISUAL, true)
    end
    raj.julianneFaking = false
    raj.phase = PHASE_BOTH
    if raj.julianne ~= nil then
        julianneArmCombat(raj.julianne, jguid)
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
    if raj.julianne ~= nil then
        raj.julianne:Talk(JULIANNE_SAY_RESURRECT)
    end
    if raj.romulo ~= nil then
        raj.romulo:CastSpell(raj.romulo, SPELL_RES_VISUAL, true)
    end
    raj.romuloFaking = false
    raj.romuloDead = false
    if raj.romulo ~= nil then
        romuloArmCombat(raj.romulo, rguid)
    end
end

romuloResurrectJulianne = function()
    local jguid = raj.julianneGuid
    if jguid == nil or not raj.julianneDead then
        return
    end
    if raj.romulo ~= nil then
        raj.romulo:Talk(ROMULO_SAY_RESURRECT)
    end
    if raj.julianne ~= nil then
        raj.julianne:CastSpell(raj.julianne, SPELL_RES_VISUAL, true)
    end
    raj.julianneFaking = false
    raj.julianneDead = false
    if raj.julianne ~= nil then
        julianneArmCombat(raj.julianne, jguid)
    end
end

RegisterCreatureEvent(ENTRY_JULIANNE, 1, julianneEnterCombat)
RegisterCreatureEvent(ENTRY_JULIANNE, 2, julianneLeaveCombat)
RegisterCreatureEvent(ENTRY_JULIANNE, 3, julianneTargetDied)
RegisterCreatureEvent(ENTRY_JULIANNE, 4, julianneDied)
RegisterCreatureEvent(ENTRY_JULIANNE, 9, julianneDamageTaken)
RegisterCreatureEvent(ENTRY_JULIANNE, 23, julianneLeaveCombat)

local function romuloLunge(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_BACKWARD_LUNGE)
    end
    schedule(guid, "lunge", math.random(15000, 30000), function()
        romuloLunge(creature, guid)
    end)
end

local function romuloDaring(creature, guid)
    creature:CastSpell(creature, SPELL_DARING)
    schedule(guid, "daring", math.random(20000, 40000), function()
        romuloDaring(creature, guid)
    end)
end

local function romuloSwathe(creature, guid)
    local target = randomPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_DEADLY_SWATHE)
    end
    schedule(guid, "swathe", math.random(15000, 25000), function()
        romuloSwathe(creature, guid)
    end)
end

local function romuloThrust(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_POISON_THRUST)
    end
    schedule(guid, "thrust", math.random(10000, 20000), function()
        romuloThrust(creature, guid)
    end)
end

romuloArmCombat = function(creature, guid)
    schedule(guid, "lunge", 15000, function()
        romuloLunge(creature, guid)
    end)
    schedule(guid, "daring", 20000, function()
        romuloDaring(creature, guid)
    end)
    schedule(guid, "swathe", 25000, function()
        romuloSwathe(creature, guid)
    end)
    schedule(guid, "thrust", 10000, function()
        romuloThrust(creature, guid)
    end)
end

local function romuloPretendDie(creature, guid)
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
    if raj.romuloGuid == guid then
        raj.romuloFaking = false
        raj.julianneDead = false
        raj.phase = PHASE_ROMULO
    end
end

local function romuloTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
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
    if raj.romuloFaking then
        return false, 0
    end
    if damage < creature:GetHealth() then
        return false, damage
    end
    if raj.phase == PHASE_ROMULO then
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
            -- C++ kills Julianne for real here; no cross-creature kill
            -- bridge, so she stays faking (documented). Romulo dies.
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

RegisterCreatureEvent(ENTRY_ROMULO, 1, romuloEnterCombat)
RegisterCreatureEvent(ENTRY_ROMULO, 2, romuloLeaveCombat)
RegisterCreatureEvent(ENTRY_ROMULO, 3, romuloTargetDied)
RegisterCreatureEvent(ENTRY_ROMULO, 4, romuloDied)
RegisterCreatureEvent(ENTRY_ROMULO, 9, romuloDamageTaken)
RegisterCreatureEvent(ENTRY_ROMULO, 23, romuloLeaveCombat)
