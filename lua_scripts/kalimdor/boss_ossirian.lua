-- Ruins of Ahn'Qiraj: Ossirian the Unscarred — Lua port of
-- src/server/scripts/Kalimdor/RuinsOfAhnQiraj/boss_ossirian.cpp
-- (boss_ossirianAI : public BossAI(creature, DATA_OSSIRIAN);
-- GetAI via GetAQ20AI<boss_ossirianAI>(creature) (AQ20ScriptName
-- "instance_ruins_of_ahnqiraj" gate); AddSC_boss_ossirian at end
-- registers boss_ossirian + go_ossirian_crystal; kalimdor loader
-- decl 84 / call 197 per kalimdor_script_loader.cpp — sixth
-- "// Ruins of ahn'qiraj" loader group, after ayamiss, before the
-- instance script).
-- Whole-server-tree quoted-name grep confirms the cpp as the sole
-- source of "boss_ossirian" (loader decl/call lines only otherwise).
-- Entry: ruins_of_ahnqiraj.h:46 names NPC_OSSIRIAN = 15339; the
-- creature_template ScriptName binding stays DB-side.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 14 OnSpellHit, 23 OnReset (no event-9
-- handler: C++ has no health latch). Timers via CreateLuaEvent;
-- melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (the self-contained in-combat legs):
-- - Engage: schedule SILENCE (30s), CYCLONE (20s), STOMP (30s);
--   self-cast SUPREME 25176; Talk(SAY_AGGRO 2); 5x sand-vortex
--   summons (NPC_SAND_VORTEX 15428, GetRandomPoint(RoomCenter, 165yd)
--   twinned in-Lua) each with triggered self-cast SAND_STORM 25160
--   (live SummonCreature bridge 2026-10-22; weather arm has no
--   bridge, crystal machine documented below).
-- - EVENT_SILENCE: Curse of Tongues 25195 via DoCastAOE lowering to
--   self-cast (C++ DoCastAOE = DoCast(nullptr, spell); the spell's
--   own AoE targeting resolves the victims — selin_fireheart
--   convention), reschedule 20-30s.
-- - EVENT_CYCLONE: DoCastVictim 25189 (jeklik GetVictim + CastSpell
--   convention), reschedule 20s.
-- - EVENT_STOMP: self-cast 25188, reschedule 30s.
-- - Supreme reapply (5s timer, C++ UpdateAI's per-tick HasAura
--   check): if not HasAura(SUPREME 25176) and not HasAura(any of
--   the 5 weaknesses) -> self-cast SUPREME + Talk(SAY_SUPREME 0)
--   (netherspite HasAura convention).
-- - No-kiting (5s timer, C++ per-tick distance check): victim
--   distance 60-120 (moroes GetDistance convention) ->
--   DoCastVictim(SPELL_SUMMON 20477).
-- - KilledUnit (event 3): Talk(SAY_SLAY 3), no target gate (C++
--   ignores the victim).
-- - SpellHit (event 14, midnight convention): weakness spell ID ->
--   RemoveAura(SUPREME 25176) (netherspite RemoveAura convention),
--   crystal Delete via caster:FindNearestGameObject(180619, 5.0) +
--   RemoveFromWorld (dynamic-GO leg, lua_objects.go) in C++ order,
--   caster Despawn, then SpawnNextCrystal (below).
-- Unmodeled (documented-only, no bridges):
-- - Weather (WEATHER_STATE_HEAVY_SANDSTORM): no bridge.
-- - Crystal/trigger summons (SpawnNextCrystal): PORTED (see spawnNextCrystal
--   below); the weakness-cast chain that drives SpellHit stays out —
--   crystal gossip -> boss DoAction -> trigger cast has no bridge
--   (GO handler cannot reach TriggerGUID; see the go_ossirian_crystal
--   note below), so the SpellHit arm fires only if a weakness spell
--   lands by other means.
-- - onReset despawns the sand-vortex summons (C++ EnterEvadeMode
--   summons.DespawnAll(), boss_ossirian.cpp:183-188).
-- - go_ossirian_crystal (GameObjectAI OnGossipHello): the GO gossip
--   hello hook IS bridged (RegisterGameObjectGossipEvent, engine/world/
--   gameobjects.go:747) — the 2026-10-18 "no bridge" claim was stale.
--   The arm stays unported on its real blockers: FindNearestCreature
--   (unbridged) + cross-AI DoAction(ACTION_TRIGGER_WEAKNESS) (unbridged).
--   go:SetLootState is bridged but is not the arm's driver.
--   Refined: FindNearestCreature IS bridged on creatures, but the GO
--   gossip handler's objects (player + GO) carry no creature lookup,
--   so the TriggerGUID receiver is unreachable from the handler; the
--   instance GetBossState(IN_PROGRESS) guard is unbridged too.
-- - MoveInLineOfSight SAY_INTRO: Eluna CREATURE_EVENT_ON_MOVE_IN_LOS (27)
--   is live (lua_creature_events.go:531-560, shaffar/vorpil/ikiss/maladaar
--   precedent) — ported below with a per-GUID one-shot SaidIntro latch
--   (C++ sets it in the AI constructor, never re-cleared; per-GUID table
--   approximates per-AI-lifetime). Return nil so the default engage
--   proceeds (C++ falls through to BossAI::MoveInLineOfSight). The
--   Go fire site is the aggro-engage branch, a subset of C++'s
--   any-unit LOS trigger — acceptable for a one-shot intro line.
-- - DoAction(ACTION_TRIGGER_WEAKNESS): cross-AI via TriggerGUID,
--   no bridge.
-- - Cleanup()/EnterEvadeMode()/JustDied() instance legs: no bridge.
-- - BossAI ctor leg DATA_OSSIRIAN and _Reset() — instance bridge,
--   standing.
local ENTRY = 15339
local ENTRY_SAND_VORTEX = 15428 -- NPC_SAND_VORTEX (ruins_of_ahnqiraj.h:50)

local SPELL_CURSE_OF_TONGUES = 25195
local SPELL_CYCLONE = 25189
local SPELL_STOMP = 25188
local SPELL_SUPREME = 25176
local SPELL_SUMMON = 20477
local SPELL_SAND_STORM = 25160 -- boss_ossirian.cpp:47 (tornado self-cast)

local SPELL_WEAKNESS_FIRE = 25177
local SPELL_WEAKNESS_FROST = 25178
local SPELL_WEAKNESS_NATURE = 25180
local SPELL_WEAKNESS_ARCANE = 25181
local SPELL_WEAKNESS_SHADOW = 25183

local WEAKNESSES = {
    SPELL_WEAKNESS_FIRE,
    SPELL_WEAKNESS_FROST,
    SPELL_WEAKNESS_NATURE,
    SPELL_WEAKNESS_ARCANE,
    SPELL_WEAKNESS_SHADOW,
}

local ENTRY_OSSIRIAN_TRIGGER = 15590 -- NPC_OSSIRIAN_TRIGGER (ruins_of_ahnqiraj.h:51)
local GO_OSSIRIAN_CRYSTAL = 180619 -- ruins_of_ahnqiraj.h:60

-- C++ CrystalCoordinates (boss_ossirian.cpp:78-91), 12 room positions
-- cycled by SpawnNextCrystal (iterator wraps at NUM_CRYSTALS).
local CRYSTAL_COORDS = {
    { -9188.4404296875, 1940.2099609375, 85.6390991210937, 3.17650008201599 },
    { -9244.41015625, 1808.97998046875, 85.6390991210937, 5.63741016387939 },
    { -9248.41015625, 1974.82995605469, 85.6390991210937, 5.89920997619629 },
    { -9282.080078125, 1887.33996582031, 85.6390991210937, 2.00712990760803 },
    { -9299.73046875, 1748.44995117187, 85.6390991210937, 1.44861996173859 },
    { -9357.8603515625, 1929.07995605469, 85.6390991210937, 1.06465005874634 },
    { -9367.169921875, 1780.89001464844, 85.6390991210937, 1.90241003036499 },
    { -9383.2900390625, 2012.68005371094, 85.6511001586914, 2.93214988708496 },
    { -9406.099609375, 1862.38000488281, 85.6390991210937, 6.2308201789856 },
    { -9407.7197265625, 1960.2099609375, 85.6390991210937, 1.11700999736786 },
    { -9432.400390625, 1782.53002929687, 85.6390991210937, 5.86430978775024 },
    { -9506.1904296875, 1865.56994628906, 85.6390991210937, 4.27606010437012 },
}

local crystalIndex = {}

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

-- C++ EVENT_SILENCE: DoCastAOE(25195), init 30s -> 20-30s
-- (DoCastAOE lowers to a self-cast; the spell's own AoE targeting
-- resolves the victims).
local function onSilence(creature, guid)
    creature:CastSpell(creature, SPELL_CURSE_OF_TONGUES)
    schedule(guid, "silence", math.random(20000, 30000), function()
        onSilence(creature, guid)
    end)
end

-- C++ EVENT_CYCLONE: DoCastVictim(25189), init 20s -> 20s.
local function onCyclone(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_CYCLONE)
    end
    schedule(guid, "cyclone", 20000, function()
        onCyclone(creature, guid)
    end)
end

-- C++ EVENT_STOMP: DoCastSelf(25188), init 30s -> 30s.
local function onStomp(creature, guid)
    creature:CastSpell(creature, SPELL_STOMP)
    schedule(guid, "stomp", 30000, function()
        onStomp(creature, guid)
    end)
end

-- C++ UpdateAI supreme leg: reapply SUPREME if neither it nor any
-- weakness is present; checked on a 5s timer (no per-tick UpdateAI
-- in Lua).
local function checkSupreme(creature, guid)
    -- C++ re-checks every UpdateAI tick; the 5s leg must stay armed
    -- even when the aura (or a weakness) is present, or the periodic
    -- check dies after the first pass.
    schedule(guid, "supreme", 5000, function()
        checkSupreme(creature, guid)
    end)
    if creature:HasAura(SPELL_SUPREME) then
        return
    end
    for _, w in ipairs(WEAKNESSES) do
        if creature:HasAura(w) then
            return
        end
    end
    creature:CastSpell(creature, SPELL_SUPREME)
    creature:Talk(0)
end

-- C++ UpdateAI no-kiting leg: victim at 60-120yd -> SPELL_SUMMON.
-- Checked on a 5s timer.
local function checkKiting(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        local dist = creature:GetDistance(victim) - creature:GetCombatReach() - (victim:GetObjectType() == "Player" and 1.5 or victim:GetCombatReach())
        if dist > 60.0 and dist < 120.0 then
            creature:CastSpell(victim, SPELL_SUMMON)
        end
    end
    schedule(guid, "kiting", 5000, function()
        checkKiting(creature, guid)
    end)
end

-- C++ SpawnNextCrystal (boss_ossirian.cpp:202-216): summon the trigger
-- at CrystalCoordinates[i], then the crystal GO from the trigger at
-- the same spot; advance the iterator only when the crystal lands
-- (C++ increments inside the crystal branch). Trigger summon type 7
-- = MANUAL_DESPAWN in the bridge's 0-based scheme (firework
-- precedent); crystal respawnTime 0 = persists (C++ Seconds::max()
-- timed despawn, effectively never).
local function spawnNextCrystal(creature, guid)
    local idx = crystalIndex[guid] or 0
    if idx >= 12 then
        idx = 0
    end
    local c = CRYSTAL_COORDS[idx + 1]
    local trigger = creature:SummonCreature(ENTRY_OSSIRIAN_TRIGGER, c[1], c[2], c[3], c[4], 7, 0)
    if trigger then
        local crystal = trigger:SummonGameObject(GO_OSSIRIAN_CRYSTAL, c[1], c[2], c[3], c[4], 0)
        if crystal then
            crystalIndex[guid] = idx + 1
        end
    end
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:CastSpell(creature, SPELL_SUPREME)
    creature:Talk(2)
    -- C++ JustEngagedWith (boss_ossirian.cpp:165-173): 5x
    -- map->SummonCreature(NPC_SAND_VORTEX, GetRandomPoint(RoomCenter,
    -- 165yd)) + triggered self-cast SPELL_SAND_STORM on each — summon
    -- bridge live 2026-10-22 (2-arg map summon = TEMPSUMMON_DEAD_
    -- DESPAWN); weather arm has no bridge. GetRandomPoint is twinned
    -- in-Lua (herod idiom: Object.cpp:1422 signed folded radius,
    -- center z kept — no ground-height model).
    for _ = 1, 5 do
        local a = math.random() * 2 * math.pi
        local r = math.random() + math.random()
        if r > 1 then
            r = r - 2
        end
        local d = 165.0 * r
        local tornado = creature:SummonCreature(ENTRY_SAND_VORTEX,
            -9343.042 + d * math.cos(a), 1923.278 + d * math.sin(a),
            85.556, 0, 6)
        if tornado then
            tornado:CastSpell(tornado, SPELL_SAND_STORM, true)
        end
    end
    -- C++ JustEngagedWith ends with SpawnNextCrystal (after the vortex
    -- summons); the weakness-cast chain that consumes the crystal
    -- stays out (crystal gossip -> boss DoAction, no bridge).
    spawnNextCrystal(creature, guid)
    schedule(guid, "silence", 30000, function()
        onSilence(creature, guid)
    end)
    schedule(guid, "cyclone", 20000, function()
        onCyclone(creature, guid)
    end)
    schedule(guid, "stomp", 30000, function()
        onStomp(creature, guid)
    end)
    schedule(guid, "supreme", 5000, function()
        checkSupreme(creature, guid)
    end)
    schedule(guid, "kiting", 5000, function()
        checkKiting(creature, guid)
    end)
end

local function onLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function onReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ Initialize (Reset, boss_ossirian.cpp:108-113): the crystal
    -- iterator restarts at 0 on every evade/reset.
    crystalIndex[guid] = nil
    -- C++ EnterEvadeMode (boss_ossirian.cpp:183-188): summons.DespawnAll()
    -- — despawn the sand-vortex summons (anubarak frost-sphere precedent)
    -- and the crystal triggers (same sweep idiom; the crystal GOs
    -- themselves linger, as in C++ where Cleanup only Uses them).
    for _, v in ipairs(creature:GetCreaturesInGrid(ENTRY_SAND_VORTEX, 500)) do
        v:Despawn()
    end
    for _, t in ipairs(creature:GetCreaturesInGrid(ENTRY_OSSIRIAN_TRIGGER, 500)) do
        t:Despawn()
    end
end

-- C++ KilledUnit: Talk(SAY_SLAY 3), victim ignored.
local function onTargetDied(event, creature, victim)
    creature:Talk(3)
end

-- C++ SpellHit (boss_ossirian.cpp:127-141), in C++ order: strip
-- SUPREME, Delete the crystal nearest the caster (5yd, dynamic-GO
-- leg), despawn the weakness-casting trigger, SpawnNextCrystal.
local function onSpellHit(event, creature, caster, spellId)
    for _, w in ipairs(WEAKNESSES) do
        if spellId == w then
            creature:RemoveAura(SPELL_SUPREME)
            if caster and caster:ToCreature() then
                local crystal = caster:FindNearestGameObject(GO_OSSIRIAN_CRYSTAL, 5.0)
                if crystal then
                    crystal:RemoveFromWorld()
                end
                caster:Despawn()
            end
            spawnNextCrystal(creature, creature:GetGUID())
            return
        end
    end
end

local function onDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

-- C++ MoveInLineOfSight: one-shot Talk(SAY_INTRO 1) with the
-- SaidIntro latch, then the default engage (boss_ossirian.cpp:218-226).
local ossirianSaidIntro = {}

local function onMoveInLOS(event, creature, unit)
    local guid = creature:GetGUID()
    if not ossirianSaidIntro[guid] then
        ossirianSaidIntro[guid] = true
        creature:Talk(1)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 3, onTargetDied)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 14, onSpellHit)
RegisterCreatureEvent(ENTRY, 23, onReset)
RegisterCreatureEvent(ENTRY, 27, onMoveInLOS)
