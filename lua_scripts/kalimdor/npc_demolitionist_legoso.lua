-- Demolitionist Legoso (Bloodmyst Isle) --
-- Lua port of src/server/scripts/Kalimdor/zone_bloodmyst_isle.cpp
-- (npc_demolitionist_legosoAI — combat rotation + the 40-phase
-- escort event machine). Zone-script unit per
-- kalimdor_script_loader.cpp order (ashenvale, azshara,
-- azuremyst_isle closed; bloodmyst_isle: npc_sironas ported,
-- webbed creature documented-only).
-- Entry (EndingTheirWorldMisc enum, verifiable from the C++
-- sources): 17982 (NPC_LEGOSO). The creature_template
-- ScriptName binding is DB-side (no TDB in this workspace).
-- Eluna creature events: 31 OnQuestAccept, 6 OnReachWP, 38
-- OnDoAction, 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset (the C++ overrides no KilledUnit/DamageTaken/
-- SpellHit — no events 3/9/14).
-- Timers via a 1s CreateLuaEvent pump (a port of UpdateAI's
-- combat section in C++ arm order — 1s granularity exact
-- for all C++ timers; the !UpdateVictim early return
-- collapses into the pump, which only runs in combat).
-- C++ DoCast default is triggered=false (vaelastrasz
-- convention): creature:CastSpell(nil, spell) = DoCastVictim.
-- Fight shape (C++-exact for all modeled arms):
-- OnEnterCombat(1): per-GUID reset to the C++ Reset()
-- values (frost shock 1s, healing surge 5s, searing totem
-- 15s, strength of earth totem 20s) + 1s scheduler pump;
-- the C++ Reset() SetCanDualWield arm has no dual-wield
-- bridge — unmodeled. Each pump tick decrements every
-- timer by 1000 (the C++ EventMap::Update(diff) pass),
-- then fires expired events in C++ arm order; a fired
-- frost/to-tem event pushes the other armed timers out by
-- 1s before they are checked (C++ EventMap::DelayEvents).
-- Pump (C++ UpdateAI arm order): frost-shock arm — 1s init
-- then 10-15s: non-triggered DoCastVictim(SPELL_FROST_SHOCK
-- 8056) (C++-exact), DelayEvents(1s), re-arm 10s+rand(5s);
-- searing-totem arm — 15s init then 110-130s: non-triggered
-- DoCast(me, SPELL_SEARING_TOTEM 38116) (C++-exact),
-- DelayEvents(1s), re-arm 110s+rand(20s); strength-of-earth-
-- totem arm — 20s init then 110-130s: non-triggered
-- DoCast(me, SPELL_STRENGTH_OF_EARTH_TOTEM 31633)
-- (C++-exact), DelayEvents(1s), re-arm 110s+rand(20s);
-- healing-surge arm — 5s init: strict self-health below 85%
-- (GetHealthPct() < 85, C++-exact) -> non-triggered
-- DoCast(self, SPELL_HEALING_SURGE 8004) (C++-exact),
-- re-arm 10s; else re-arm 2s (C++-exact; no DelayEvents
-- on the healing arm). Melee is engine-driven.
-- OnLeaveCombat(2)/OnReset(23): cancel the pump, drop
-- per-GUID state (the C++ Reset() observable remainder —
-- the event-map reset — is latched on OnEnterCombat,
-- gargolmar precedent; Eluna's On_Reset fires ahead of
-- OnDied and OnSpawn — millhouse note — so both hooks
-- land the same reset).
-- Escort machine (C++-exact for all modeled arms):
-- OnQuestAccept(31) quest 9759 -> SetData(0, player GUID
-- counter) + StartEscort(player) + MoveWaypoint() (the C++
-- Start(true, true, ...) run/instant-respawn flags have no
-- Go twin — the walk rides the per-node DB move_type, the
-- respawn exemption rides kill.go); the 100ms phase pump
-- mirrors UpdateAI's phase section (fires when the timer
-- expires, decrements only out of combat, gated on an
-- escort player like the C++ HasEscortState(STATE_ESCORT_NONE)
-- early return). WaypointReached(6) reads the DB point via
-- creature:GetCurrentWaypointId() (the C++ WaypointReached
-- (waypoint.id) twin — the event's own id arg is the node
-- index, the C++ MovementInform twin); the phase arms use
-- MoveTo (MovePoint twin — arrival is never awaited, the
-- machine advances on the timer), SummonGameObject (returns
-- the packed GUID; the detonate arms resolve it via
-- GetGameObjectByGUID + GameObject:Delete), PerformEmote
-- (HandleEmoteCommand twin), SetStandState, SetFacingTo /
-- SetFacingToObject, Talk, GetEscortPlayer, FindNearestCreature
-- (sironas legs), SetImmuneToAll(false), SetScale(3.0),
-- cross-creature DoAction, sironas:AddThreat(target, 0.001)
-- (the C++ AddThreat(sironas, 0.001, target) leg — the Go
-- bridge is self-threat only, so the call rides sironas),
-- sironas:Attack + MoveChase, GroupEventHappens on the
-- escort player.
-- Deliberate deviations: no dual-wield bridge — Reset()
-- SetCanDualWield(true) unmodeled; the waypoint walk loops
-- the DB path at its end (Go NextIdx wraps — no end-of-path
-- idle twin) where C++ EscortAI idles and drops the escort
-- state — cosmetic post-quest drift only, the quest credit
-- lands in the slain chain before it; the waypoint_data rows
-- for legoso's path live in the deployment world DB (the
-- repo world.db placeholder carries none — loadCreaturePathID
-- falls back to the spawn GUID, so a live DB without the
-- rows leaves MoveWaypoint a no-op); InterruptNonMeleeSpells
-- has no per-spell arg in Go (the sironas channel-stop arm
-- interrupts all non-melee casts — the only casts in play).
-- Zone set status: zone_bloodmyst_isle.cpp closed —
-- npc_webbed_creature documented-only (no-summon /
-- quest-credit bridges, unverifiable entry),
-- npc_sironas ported for 17678,
-- npc_demolitionist_legoso ported for 17982 (combat rotation
-- + escort event machine).

local SPELL_FROST_SHOCK = 8056
local SPELL_HEALING_SURGE = 8004
local SPELL_SEARING_TOTEM = 38116
local SPELL_STRENGTH_OF_EARTH_TOTEM = 31633

local LEGOSO_ENTRY = 17982
local NPC_SIRONAS = 17678

local QUEST_ENDING_THEIR_WORLD = 9759

local GO_DRAENEI_EXPLOSIVES_1 = 182088
local GO_DRAENEI_EXPLOSIVES_2 = 182091

local ACTION_SIRONAS_CHANNEL_START = 1
local ACTION_SIRONAS_CHANNEL_STOP = 2
local ACTION_LEGOSO_SIRONAS_KILLED = 1

local DATA_EVENT_STARTER_GUID = 0

local SAY_SIRONAS_1 = 0
local SAY_LEGOSO_1 = 0
local SAY_LEGOSO_2 = 1
local SAY_LEGOSO_3 = 2
local SAY_LEGOSO_4 = 3
local SAY_LEGOSO_5 = 4
local SAY_LEGOSO_6 = 5
local SAY_LEGOSO_7 = 6
local SAY_LEGOSO_8 = 7
local SAY_LEGOSO_9 = 8
local SAY_LEGOSO_10 = 9
local SAY_LEGOSO_11 = 10
local SAY_LEGOSO_12 = 11
local SAY_LEGOSO_13 = 12
local SAY_LEGOSO_14 = 13
local SAY_LEGOSO_15 = 14
local SAY_LEGOSO_16 = 15
local SAY_LEGOSO_17 = 16
local SAY_LEGOSO_18 = 17
local SAY_LEGOSO_19 = 18
local SAY_LEGOSO_20 = 19
local SAY_LEGOSO_21 = 20

local EMOTE_ONESHOT_NONE = 0
local EMOTE_ONESHOT_CHEER = 4
local EMOTE_ONESHOT_EXCLAMATION = 5
local EMOTE_ONESHOT_POINT = 25

local STAND_STATE_STAND = 0
local STAND_STATE_KNEEL = 1

local WP_START = 1
local WP_EXPLOSIVES_FIRST_POINT = 21
local WP_EXPLOSIVES_FIRST_PLANT = 22
local WP_EXPLOSIVES_FIRST_RUNOFF = 23
local WP_EXPLOSIVES_FIRST_DETONATE = 24
local WP_DEBUG_1 = 25
local WP_DEBUG_2 = 26
local WP_SIRONAS_HILL = 33
local WP_EXPLOSIVES_SECOND_BATTLEROAR = 35
local WP_EXPLOSIVES_SECOND_PLANT = 39
local WP_EXPLOSIVES_SECOND_DETONATE = 40

local PHASE_NONE = 0
local PHASE_CONTINUE = -1
local PHASE_WP_26 = 1
local PHASE_WP_22 = 2
local PHASE_PLANT_FIRST_KNEEL = 3
local PHASE_PLANT_FIRST_STAND = 4
local PHASE_PLANT_FIRST_WORK = 5
local PHASE_PLANT_FIRST_FINISH = 6
local PHASE_PLANT_FIRST_TIMER_1 = 7
local PHASE_PLANT_FIRST_TIMER_2 = 8
local PHASE_PLANT_FIRST_TIMER_3 = 9
local PHASE_PLANT_FIRST_DETONATE = 10
local PHASE_PLANT_FIRST_SPEECH = 11
local PHASE_PLANT_FIRST_ROTATE = 12
local PHASE_PLANT_FIRST_POINT = 13
local PHASE_FEEL_SIRONAS_1 = 14
local PHASE_FEEL_SIRONAS_2 = 15
local PHASE_MEET_SIRONAS_ROAR = 16
local PHASE_MEET_SIRONAS_TURN = 17
local PHASE_MEET_SIRONAS_SPEECH = 18
local PHASE_PLANT_SECOND_KNEEL = 19
local PHASE_PLANT_SECOND_SPEECH = 20
local PHASE_PLANT_SECOND_STAND = 21
local PHASE_PLANT_SECOND_FINISH = 22
local PHASE_PLANT_SECOND_WAIT = 23
local PHASE_PLANT_SECOND_TIMER_1 = 24
local PHASE_PLANT_SECOND_TIMER_2 = 25
local PHASE_PLANT_SECOND_TIMER_3 = 26
local PHASE_PLANT_SECOND_DETONATE = 27
local PHASE_FIGHT_SIRONAS_STOP = 28
local PHASE_FIGHT_SIRONAS_SPEECH_1 = 29
local PHASE_FIGHT_SIRONAS_SPEECH_2 = 30
local PHASE_FIGHT_SIRONAS_START = 31
local PHASE_SIRONAS_SLAIN_SPEECH_1 = 32
local PHASE_SIRONAS_SLAIN_EMOTE_1 = 33
local PHASE_SIRONAS_SLAIN_EMOTE_2 = 34
local PHASE_SIRONAS_SLAIN_SPEECH_2 = 35

-- ExplosivesPos (zone_bloodmyst_isle.cpp:231-249): the two
-- 5-charge plant sites, orientation 0 (C++ QuaternionData()).
local EXPLOSIVES_POS_1 = {
    { -1954.946, -10654.714, 110.448 },
    { -1956.331, -10654.494, 110.869 },
    { -1955.906, -10656.221, 110.791 },
    { -1957.294, -10656.000, 111.219 },
    { -1954.462, -10656.451, 110.404 },
}
local EXPLOSIVES_POS_2 = {
    { -1915.137, -10583.651, 178.365 },
    { -1914.006, -10582.964, 178.471 },
    { -1912.717, -10582.398, 178.658 },
    { -1915.056, -10582.251, 178.162 },
    { -1913.883, -10581.778, 178.346 },
}

local legosoState = {}
local legosoPump = {}
local legosoEscort = {}
local legosoEscortPump = {}

local TIMER_KEYS = { "frostTimer", "searingTimer", "strengthTimer", "healingTimer" }

local function cancelEscortPump(guid)
    local id = legosoEscortPump[guid]
    if id then
        RemoveEventById(id)
        legosoEscortPump[guid] = nil
    end
end

local function cancelPump(guid)
    local id = legosoPump[guid]
    if id then
        RemoveEventById(id)
        legosoPump[guid] = nil
    end
end

-- C++ Reset() (called on evade): fullReset — the observable
-- remainder (event-map reset) is re-latched on OnEnterCombat
-- (gargolmar precedent). The escort phase machine dies with
-- Reset (C++ _phase = PHASE_NONE, _moveTimer = 0,
-- _eventStarterGuidLow = 0), so its pump, state and starter
-- GUID clear here too.
local function fullReset(creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    cancelEscortPump(guid)
    legosoState[guid] = nil
    legosoEscort[guid] = nil
    creature:SetData(DATA_EVENT_STARTER_GUID, 0)
end

-- C++ EventMap::DelayEvents(1s): push every other armed
-- timer out by 1000ms, preserving remaining time.
local function delayOthers(st, except)
    for _, key in ipairs(TIMER_KEYS) do
        if key ~= except then
            st[key] = st[key] + 1000
        end
    end
end

-- C++ UpdateAI combat section in arm order — 1s granularity
-- exact for all C++ timers; the !UpdateVictim early return
-- collapses into the pump (it only runs in combat). Each
-- tick decrements every timer (the EventMap::Update(diff)
-- pass), then expired events fire in arm order with their
-- DelayEvents applied before the next event is checked.
local function combatTick(creature, guid)
    local st = legosoState[guid]
    if not st then
        return
    end

    for _, key in ipairs(TIMER_KEYS) do
        st[key] = st[key] - 1000
    end

    -- Frost shock arm — 1s init then 10-15s: non-triggered
    -- DoCastVictim, DelayEvents(1s), re-arm 10s+rand(5s)
    -- regardless (C++-exact).
    if st.frostTimer <= 0 then
        creature:CastSpell(nil, SPELL_FROST_SHOCK)
        delayOthers(st, "frostTimer")
        st.frostTimer = 10000 + math.random(0, 5000)
    end

    -- Searing totem arm — 15s init then 110-130s: non-
    -- triggered DoCast(self), DelayEvents(1s), re-arm
    -- 110s+rand(20s) regardless (C++-exact).
    if st.searingTimer <= 0 then
        creature:CastSpell(creature, SPELL_SEARING_TOTEM)
        delayOthers(st, "searingTimer")
        st.searingTimer = 110000 + math.random(0, 20000)
    end

    -- Strength of earth totem arm — 20s init then 110-130s:
    -- non-triggered DoCast(self), DelayEvents(1s), re-arm
    -- 110s+rand(20s) regardless (C++-exact).
    if st.strengthTimer <= 0 then
        creature:CastSpell(creature, SPELL_STRENGTH_OF_EARTH_TOTEM)
        delayOthers(st, "strengthTimer")
        st.strengthTimer = 110000 + math.random(0, 20000)
    end

    -- Healing surge arm — 5s init, no DelayEvents: C++ compares the
    -- FLOAT me->GetHealthPct() < 85 (zone_bloodmyst_isle.cpp:452), so
    -- the float GetHealthPct() < 85 form is C++-exact here — NOT the
    -- integer-threshold form (maulgar/ironaya only applies to C++
    -- HealthBelowPct). Non-triggered DoCast(self), re-arm 10s; else
    -- re-arm 2s (C++-exact). The escort-
    -- player arm rides creature:GetEscortPlayer()
    -- (lua_creature_events.go) — nil escort falls through
    -- to the 2s re-arm, C++-exact.
    if st.healingTimer <= 0 then
        if creature:GetHealthPct() < 85 then
            creature:CastSpell(creature, SPELL_HEALING_SURGE)
            st.healingTimer = 10000
        else
            local escort = creature:GetEscortPlayer()
            if escort ~= nil and escort:GetHealthPct() < 85 then
                creature:CastSpell(escort, SPELL_HEALING_SURGE)
                st.healingTimer = 10000
            else
                st.healingTimer = 2000
            end
        end
    end
end

-- C++ Reset() values land on OnEnterCombat: frost shock 1s,
-- healing surge 5s, searing totem 15s, strength of earth
-- totem 20s.
local function onEnterCombat(_, creature)
    local guid = creature:GetGUID()
    cancelPump(guid)
    legosoState[guid] = {
        frostTimer = 1000,
        healingTimer = 5000,
        searingTimer = 15000,
        strengthTimer = 20000,
    }
    legosoPump[guid] = CreateLuaEvent(function()
        combatTick(creature, guid)
    end, 1000, 0)
end

-- The C++ JustDied override is empty; the hook drops
-- per-GUID state like the other cleanup hooks.
local function onDied(_, creature)
    fullReset(creature)
end

-- C++ Reset() (called on evade): fullReset — see above.
local function onLeaveCombat(_, creature)
    fullReset(creature)
end

-- Escort phase machine (zone_bloodmyst_isle.cpp:473-769):
-- one arm per _phase value, in C++ switch order. Each arm
-- runs when _moveTimer expires and seeds the next phase +
-- timer; the pump below supplies the 100ms UpdateAI tick.

local function escortState(guid)
    local es = legosoEscort[guid]
    if not es then
        es = { phase = PHASE_NONE, moveTimer = 0, explosives = {} }
        legosoEscort[guid] = es
    end
    return es
end

local function nearestSironas(creature)
    return creature:FindNearestCreature(NPC_SIRONAS, 533.33)
end

local function detonateExplosives(creature, es)
    for _, guid in ipairs(es.explosives) do
        local go = creature:GetGameObjectByGUID(guid)
        if go then
            go:Delete()
        end
    end
    es.explosives = {}
end

local function summonExplosives(creature, es, entry, positions)
    es.explosives = {}
    for _, pos in ipairs(positions) do
        local guid = creature:SummonGameObject(entry, pos[1], pos[2], pos[3], 0, 0)
        if guid then
            es.explosives[#es.explosives + 1] = guid
        end
    end
end

local function runPhase(creature, guid, es)
    local phase = es.phase
    if phase == PHASE_WP_26 then -- debug skip path to point 26
        creature:MoveTo(WP_DEBUG_2, -2021.77, -10648.8, 129.903)
        es.moveTimer = 2000
        es.phase = PHASE_CONTINUE
    elseif phase == PHASE_CONTINUE then -- continue escort
        creature:SetEscortPaused(false)
        es.moveTimer = 0
        es.phase = PHASE_NONE
    elseif phase == PHASE_WP_22 then -- debug skip path to point 22
        creature:MoveTo(WP_EXPLOSIVES_FIRST_PLANT, -1958.026, -10660.465, 111.547)
        creature:Talk(SAY_LEGOSO_3)
        es.moveTimer = 2000
        es.phase = PHASE_PLANT_FIRST_KNEEL
    elseif phase == PHASE_PLANT_FIRST_KNEEL then -- plant first explosives stage 1 kneel
        creature:SetStandState(STAND_STATE_KNEEL)
        es.moveTimer = 10000
        es.phase = PHASE_PLANT_FIRST_STAND
    elseif phase == PHASE_PLANT_FIRST_STAND then -- plant first explosives stage 1 stand
        creature:SetStandState(STAND_STATE_STAND)
        es.moveTimer = 500
        es.phase = PHASE_PLANT_FIRST_WORK
    elseif phase == PHASE_PLANT_FIRST_WORK then -- plant first explosives stage 2 work
        creature:Talk(SAY_LEGOSO_4)
        es.moveTimer = 17500
        es.phase = PHASE_PLANT_FIRST_FINISH
    elseif phase == PHASE_PLANT_FIRST_FINISH then -- plant first explosives finish
        summonExplosives(creature, es, GO_DRAENEI_EXPLOSIVES_1, EXPLOSIVES_POS_1)
        creature:PerformEmote(EMOTE_ONESHOT_NONE) -- reset anim state
        -- force runoff movement so he will not screw up next waypoint
        creature:MoveTo(WP_EXPLOSIVES_FIRST_RUNOFF, -1955.6, -10669.8, 110.65)
        creature:Talk(SAY_LEGOSO_5)
        es.moveTimer = 1500
        es.phase = PHASE_CONTINUE
    elseif phase == PHASE_PLANT_FIRST_TIMER_1 then -- first explosives detonate timer 1
        creature:Talk(SAY_LEGOSO_6)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_FIRST_TIMER_2
    elseif phase == PHASE_PLANT_FIRST_TIMER_2 then -- first explosives detonate timer 2
        creature:Talk(SAY_LEGOSO_7)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_FIRST_TIMER_3
    elseif phase == PHASE_PLANT_FIRST_TIMER_3 then -- first explosives detonate timer 3
        creature:Talk(SAY_LEGOSO_8)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_FIRST_DETONATE
    elseif phase == PHASE_PLANT_FIRST_DETONATE then -- first explosives detonate finish
        detonateExplosives(creature, es)
        creature:PerformEmote(EMOTE_ONESHOT_CHEER)
        es.moveTimer = 2000
        es.phase = PHASE_PLANT_FIRST_SPEECH
    elseif phase == PHASE_PLANT_FIRST_SPEECH then -- after detonation 1 speech
        creature:Talk(SAY_LEGOSO_9)
        es.moveTimer = 4000
        es.phase = PHASE_PLANT_FIRST_ROTATE
    elseif phase == PHASE_PLANT_FIRST_ROTATE then -- after detonation 1 rotate to next point
        creature:SetFacingTo(2.272)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_FIRST_POINT
    elseif phase == PHASE_PLANT_FIRST_POINT then -- after detonation 1 send point anim and go on to next point
        creature:PerformEmote(EMOTE_ONESHOT_POINT)
        es.moveTimer = 2000
        es.phase = PHASE_CONTINUE
    elseif phase == PHASE_FEEL_SIRONAS_1 then -- legoso exclamation before sironas 1.1
        creature:Talk(SAY_LEGOSO_10)
        es.moveTimer = 4000
        es.phase = PHASE_FEEL_SIRONAS_2
    elseif phase == PHASE_FEEL_SIRONAS_2 then -- legoso exclamation before sironas 1.2
        creature:Talk(SAY_LEGOSO_11)
        es.moveTimer = 4000
        es.phase = PHASE_CONTINUE
    elseif phase == PHASE_MEET_SIRONAS_ROAR then -- legoso exclamation before sironas 2.1
        creature:Talk(SAY_LEGOSO_12)
        es.moveTimer = 4000
        es.phase = PHASE_MEET_SIRONAS_TURN
    elseif phase == PHASE_MEET_SIRONAS_TURN then -- legoso exclamation before sironas 2.2
        local player = creature:GetEscortPlayer()
        if player then
            creature:SetFacingToObject(player)
        end
        es.moveTimer = 1000
        es.phase = PHASE_MEET_SIRONAS_SPEECH
    elseif phase == PHASE_MEET_SIRONAS_SPEECH then -- legoso exclamation before sironas 2.3
        creature:Talk(SAY_LEGOSO_13)
        es.moveTimer = 7000
        es.phase = PHASE_CONTINUE
    elseif phase == PHASE_PLANT_SECOND_KNEEL then -- plant second explosives stage 1 kneel
        creature:SetStandState(STAND_STATE_KNEEL)
        es.moveTimer = 11000
        es.phase = PHASE_PLANT_SECOND_SPEECH
    elseif phase == PHASE_PLANT_SECOND_SPEECH then -- plant second explosives stage 2 kneel
        creature:Talk(SAY_LEGOSO_14)
        es.moveTimer = 13000
        es.phase = PHASE_PLANT_SECOND_STAND
    elseif phase == PHASE_PLANT_SECOND_STAND then -- plant second explosives finish
        creature:SetStandState(STAND_STATE_STAND)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_SECOND_FINISH
    elseif phase == PHASE_PLANT_SECOND_FINISH then -- plant second explosives finish - create explosives
        summonExplosives(creature, es, GO_DRAENEI_EXPLOSIVES_2, EXPLOSIVES_POS_2)
        creature:Talk(SAY_LEGOSO_15)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_SECOND_WAIT
    elseif phase == PHASE_PLANT_SECOND_WAIT then -- plant second explosives finish - proceed to next point
        es.moveTimer = 1000
        es.phase = PHASE_CONTINUE
    elseif phase == PHASE_PLANT_SECOND_TIMER_1 then -- second explosives detonate timer 1
        creature:Talk(SAY_LEGOSO_16)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_SECOND_TIMER_2
    elseif phase == PHASE_PLANT_SECOND_TIMER_2 then -- second explosives detonate timer 2
        creature:Talk(SAY_LEGOSO_17)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_SECOND_TIMER_3
    elseif phase == PHASE_PLANT_SECOND_TIMER_3 then -- second explosives detonate timer 3
        creature:Talk(SAY_LEGOSO_18)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_SECOND_DETONATE
    elseif phase == PHASE_PLANT_SECOND_DETONATE then -- second explosives detonate finish
        detonateExplosives(creature, es)
        local sironas = nearestSironas(creature)
        if sironas then
            sironas:SetImmuneToAll(false)
            creature:SetFacingToObject(sironas)
        end
        es.moveTimer = 1000
        es.phase = PHASE_FIGHT_SIRONAS_STOP
    elseif phase == PHASE_FIGHT_SIRONAS_STOP then -- sironas channel stop
        local sironas = nearestSironas(creature)
        if sironas then
            sironas:DoAction(ACTION_SIRONAS_CHANNEL_STOP)
        end
        es.moveTimer = 1000
        es.phase = PHASE_FIGHT_SIRONAS_SPEECH_1
    elseif phase == PHASE_FIGHT_SIRONAS_SPEECH_1 then -- sironas exclamation before aggro
        local sironas = nearestSironas(creature)
        if sironas then
            sironas:Talk(SAY_SIRONAS_1)
        end
        es.moveTimer = 1000
        es.phase = PHASE_FIGHT_SIRONAS_SPEECH_2
    elseif phase == PHASE_FIGHT_SIRONAS_SPEECH_2 then -- legoso exclamation before aggro
        local sironas = nearestSironas(creature)
        if sironas then
            sironas:SetScale(3.0)
        end
        creature:Talk(SAY_LEGOSO_19)
        es.moveTimer = 1000
        es.phase = PHASE_FIGHT_SIRONAS_START
    elseif phase == PHASE_FIGHT_SIRONAS_START then -- legoso exclamation at aggro
        local sironas = nearestSironas(creature)
        if sironas then
            local target = creature:GetEscortPlayer()
            if not target then
                target = creature
            end
            sironas:AddThreat(target:GetGUID(), 0.001)
            sironas:Attack(target, true)
            sironas:MoveChase(target)
        end
        es.moveTimer = 10000
        es.phase = PHASE_CONTINUE
    elseif phase == PHASE_SIRONAS_SLAIN_SPEECH_1 then -- legoso exclamation after battle - stage 1.1
        creature:Talk(SAY_LEGOSO_20)
        es.moveTimer = 2000
        es.phase = PHASE_SIRONAS_SLAIN_EMOTE_1
    elseif phase == PHASE_SIRONAS_SLAIN_EMOTE_1 then -- legoso exclamation after battle - stage 1.2
        creature:PerformEmote(EMOTE_ONESHOT_EXCLAMATION)
        es.moveTimer = 2000
        es.phase = PHASE_SIRONAS_SLAIN_EMOTE_2
    elseif phase == PHASE_SIRONAS_SLAIN_EMOTE_2 then -- legoso exclamation after battle - stage 1.3
        local player = creature:GetEscortPlayer()
        if player then
            player:GroupEventHappens(QUEST_ENDING_THEIR_WORLD, creature)
        end
        creature:PerformEmote(EMOTE_ONESHOT_CHEER)
        es.moveTimer = 5000
        es.phase = PHASE_SIRONAS_SLAIN_SPEECH_2
    elseif phase == PHASE_SIRONAS_SLAIN_SPEECH_2 then -- legoso exclamation after battle - stage 2
        creature:Talk(SAY_LEGOSO_21)
        es.moveTimer = 30000
        es.phase = PHASE_CONTINUE
    end
end

-- C++ UpdateAI phase section (zone_bloodmyst_isle.cpp:473-480):
-- fires the armed phase when _moveTimer expires, decrements
-- only out of combat (the timer freezes while engaged).
local function phaseTick(creature, guid)
    local es = legosoEscort[guid]
    if not es or es.phase == PHASE_NONE then
        return
    end
    if creature:GetEscortPlayer() == nil then
        return
    end
    if es.moveTimer <= 100 then
        runPhase(creature, guid, es)
    elseif not creature:IsInCombat() then
        es.moveTimer = es.moveTimer - 100
    end
end

local function startEscortPump(guid, creature)
    cancelEscortPump(guid)
    escortState(guid)
    legosoEscortPump[guid] = CreateLuaEvent(function()
        phaseTick(creature, guid)
    end, 100, 0)
end

-- C++ OnQuestAccept (zone_bloodmyst_isle.cpp:375-382): quest
-- 9759 -> SetData(DATA_EVENT_STARTER_GUID, player GUID
-- counter) + Start(true, true, player GUID, quest).
local function onQuestAccept(_, player, creature, quest)
    if not quest or quest:GetId() ~= QUEST_ENDING_THEIR_WORLD then
        return
    end
    creature:SetData(DATA_EVENT_STARTER_GUID, player:GetGUIDLow())
    creature:StartEscort(player)
    creature:MoveWaypoint()
    startEscortPump(creature:GetGUID(), creature)
end

-- C++ WaypointReached (zone_bloodmyst_isle.cpp:725-823): the
-- waypoint id is the DB point (GetCurrentWaypointId), not the
-- event's node index. Every arm requires the escort player
-- (the C++ GetPlayerForEscort() nil-guard at the top).
local function onReachWP(_, creature)
    local player = creature:GetEscortPlayer()
    if not player then
        return
    end
    local guid = creature:GetGUID()
    local es = escortState(guid)
    local wp = creature:GetCurrentWaypointId()
    if wp == WP_START then
        creature:SetEscortPaused(true)
        creature:SetFacingToObject(player)
        creature:Talk(SAY_LEGOSO_1)
        es.moveTimer = 2500
        es.phase = PHASE_CONTINUE
    elseif wp == WP_EXPLOSIVES_FIRST_POINT then
        creature:SetEscortPaused(true)
        creature:Talk(SAY_LEGOSO_2)
        es.moveTimer = 8000
        es.phase = PHASE_WP_22
    elseif wp == WP_EXPLOSIVES_FIRST_PLANT then
        creature:SetFacingTo(1.46)
    elseif wp == WP_EXPLOSIVES_FIRST_DETONATE then
        creature:SetEscortPaused(true)
        creature:SetFacingTo(1.05)
        es.moveTimer = 1000
        es.phase = PHASE_PLANT_FIRST_TIMER_1
    elseif wp == WP_DEBUG_1 then
        creature:SetEscortPaused(true)
        es.moveTimer = 500
        es.phase = PHASE_WP_26
    elseif wp == WP_SIRONAS_HILL then
        creature:SetEscortPaused(true)
        -- Find Sironas and make it respawn if needed
        local sironas = nearestSironas(creature)
        if sironas then
            if sironas:IsDead() then
                sironas:Respawn()
            end
            sironas:DoAction(ACTION_SIRONAS_CHANNEL_START)
            creature:SetFacingToObject(sironas)
        end
        es.moveTimer = 1000
        es.phase = PHASE_FEEL_SIRONAS_1
    elseif wp == WP_EXPLOSIVES_SECOND_BATTLEROAR then
        creature:SetEscortPaused(true)
        es.moveTimer = 200
        es.phase = PHASE_MEET_SIRONAS_ROAR
    elseif wp == WP_EXPLOSIVES_SECOND_PLANT then
        creature:SetEscortPaused(true)
        es.moveTimer = 500
        es.phase = PHASE_PLANT_SECOND_KNEEL
    elseif wp == WP_EXPLOSIVES_SECOND_DETONATE then
        creature:SetEscortPaused(true)
        creature:SetFacingTo(5.7)
        es.moveTimer = 2000
        es.phase = PHASE_PLANT_SECOND_TIMER_1
    end
end

-- C++ DoAction(ACTION_LEGOSO_SIRONAS_KILLED)
-- (zone_bloodmyst_isle.cpp:825-833): seeds the slain chain —
-- 5s -> Talk(SAY_LEGOSO_20), 2s -> exclamation emote, 2s ->
-- quest credit + cheer emote, 5s -> Talk(SAY_LEGOSO_21), 30s
-- -> escort resume. The chain rides the phase pump (the C++
-- _phase/_moveTimer fields), so the pump must be running —
-- it starts on quest accept and dies on Reset.
local function onDoAction(event, creature, action)
    if action ~= ACTION_LEGOSO_SIRONAS_KILLED then
        return
    end
    local guid = creature:GetGUID()
    local es = escortState(guid)
    es.phase = PHASE_SIRONAS_SLAIN_SPEECH_1
    es.moveTimer = 5000
    if not legosoEscortPump[guid] then
        startEscortPump(guid, creature)
    end
end

RegisterCreatureEvent(LEGOSO_ENTRY, 31, onQuestAccept)
RegisterCreatureEvent(LEGOSO_ENTRY, 6, onReachWP)
RegisterCreatureEvent(LEGOSO_ENTRY, 38, onDoAction)

RegisterCreatureEvent(LEGOSO_ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(LEGOSO_ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(LEGOSO_ENTRY, 4, onDied)
-- Eluna's On_Reset fires ahead of OnDied and OnSpawn, so
-- this lands the same Reset() reset as the evade hook.
RegisterCreatureEvent(LEGOSO_ENTRY, 23, onLeaveCombat)
