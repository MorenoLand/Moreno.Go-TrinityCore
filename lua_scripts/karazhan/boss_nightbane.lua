-- Nightbane (Karazhan) — Lua port of
-- src/server/scripts/EasternKingdoms/Karazhan/boss_nightbane.cpp
-- Creature entry: 17225 (karazhan.h NPC_NIGHTBANE; TDB creature_template
-- ScriptName "boss_nightbane"). Instance data: DATA_NIGHTBANE = 11,
-- DATA_MASTERS_TERRACE_DOOR_1 = 27, DATA_MASTERS_TERRACE_DOOR_2 = 28,
-- DATA_GO_BLACKENED_URN = 30 (karazhan.h:41,56-57,59); the encounter only
-- touches instance state for the terrace doors, the urn IN_USE flag, and
-- the BossAI admission — no Go instance-script model, so those arms are
-- skipped and DATA_NIGHTBANE is admission-only via the luaBossAI shim in
-- engine/world/boss_ai.go.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 9 OnDamageTaken (75/50/25% fly-phase arms + fly-phase never-die clamp),
-- 23 OnReset. Timers via CreateLuaEvent; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Fight shape: ground phase (Cleave / Tail Sweep / Bellowing Roar /
-- Charred Earth / Smoldering Breath / Distracting Ash) until a DamageTaken
-- arm drops below 75%, 50%, 25% pre-damage health (flyCount 0/1/2) —
-- then the fly phase: Rain of Bones after a breath emote, Smoking Blast
-- 1400ms, Smoking Blast target-cast 5-7s, landing ~33s later, and the
-- ground phase is re-armed on touchdown.
-- Deviations from C++: the engine has no movement model (no spline
-- chains, MovePoint, or gravity), so the intro flight path (ACTION_SUMMON
-- / DoAction) and the whole fly-phase waypoint choreography are
-- compressed into timer chains — the pre-fly waypoint choice (nearest of
-- the three fly positions) fires 1ms after liftoff, the fly-to-landing
-- spline travel is approximated as two 5s delays; liftoff/land emotes
-- port via creature:PerformEmote (254/293), while gravity flips,
-- react-state flips, AttackStop, and InterruptNonMeleeSpells have
-- no bridge. No gameobject event dispatch
-- exists in the engine, so the go_blackened_urn gossip arm (entry 194092)
-- and the intro (EMOTE_SUMMON, IMMUNE_TO_PC until landing, DoZoneInCombat)
-- are not modeled: the port starts directly at the landing combat state.
-- Rain of Bones (37098) is cast on the random target only — its
-- AuraScript periodic skeleton-summon arm (SPELL_SUMMON_SKELETON 30170,
-- every 5th tick) has no bridge; AuraScript check handlers are a standing
-- unmodeled area. No threat model, so ResetThreatList is skipped. Tail
-- Sweep's HasInArc(PI) behind-target gate is skipped: the bridge exposes
-- no reliable facing, so the random target is cast on directly. No
-- UNIT_STATE_CASTING model, so timers fire unconditionally.

local ENTRY_NIGHTBANE = 17225

local EMOTE_SUMMON, YELL_AGGRO = 0, 1
local YELL_FLY_PHASE, YELL_LAND_PHASE, EMOTE_BREATH = 2, 3, 4

local SPELL_BELLOWING_ROAR = 36922
local SPELL_CHARRED_EARTH = 30129
local SPELL_CLEAVE = 30131
local SPELL_DISTRACTING_ASH = 30130
local SPELL_RAIN_OF_BONES = 37098
local SPELL_SMOKING_BLAST = 30128
local SPELL_SMOKING_BLAST_T = 37057
local SPELL_SMOLDERING_BREATH = 30210
local SPELL_TAIL_SWEEP = 25653

local PHASE_GROUND, PHASE_FLY = 1, 2

local timers = {}
local nightbaneState = {}

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

local function cancelPrefix(guid, prefix)
    local per = timers[guid]
    if not per then
        return
    end
    for key, id in pairs(per) do
        if string.sub(key, 1, #prefix) == prefix then
            RemoveEventById(id)
            per[key] = nil
        end
    end
end

local function randomAlivePlayer(creature)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, p in ipairs(GetPlayersInWorld()) do
        if p:GetMapId() == mapId and p:GetInstanceId() == instanceId
                and not p:IsDead() then
            candidates[#candidates + 1] = p
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function onCleave(creature, guid)
    creature:CastSpell(nil, SPELL_CLEAVE)
    schedule(guid, "g_cleave", {6000, 15000}, function() onCleave(creature, guid) end)
end

local function onTailSweep(creature, guid)
    local target = randomAlivePlayer(creature)
    if target ~= nil then
        -- C++ HasInArc(PI, target) behind-gate has no facing bridge.
        creature:CastSpell(target, SPELL_TAIL_SWEEP)
    end
    schedule(guid, "g_tailsweep", {20000, 30000}, function() onTailSweep(creature, guid) end)
end

local function onBellowingRoar(creature, guid)
    if nightbaneState[guid] == nil then
        return
    end
    creature:CastSpell(creature, SPELL_BELLOWING_ROAR)
    -- C++ arms this once (no repeat); a fly phase or evade cancels it.
end

local function onCharredEarth(creature, guid)
    local target = randomAlivePlayer(creature)
    if target ~= nil then
        creature:CastSpell(target, SPELL_CHARRED_EARTH)
    end
    schedule(guid, "g_charredearth", {18000, 21000}, function() onCharredEarth(creature, guid) end)
end

local function onSmolderingBreath(creature, guid)
    creature:CastSpell(nil, SPELL_SMOLDERING_BREATH)
    schedule(guid, "g_smoldering", {28000, 40000}, function() onSmolderingBreath(creature, guid) end)
end

local function onDistractingAsh(creature, guid)
    local st = nightbaneState[guid]
    if st == nil then
        return
    end
    local target = randomAlivePlayer(creature)
    if target ~= nil then
        creature:CastSpell(target, SPELL_DISTRACTING_ASH)
    end
    -- C++ arms this once (no repeat); a fly phase or evade cancels it.
end

-- C++ SetupGroundPhase. The 4-arg ScheduleEvent(id, min, max, GROUP)
-- overload resolves to the (minTime, maxTime, group) form
-- (EventMap.h:135) and draws randtime(min, max) = urand inclusive
-- (EventMap.cpp:47-49), so the initial schedules are ranges here, not
-- fixed delays. Cleave's range bottoms at 0s — CreateLuaEvent floors at
-- 1ms (runtime.go: createLuaEvent rejects minDelay < 1), so {1,15000}
-- is the exact representable form. Bellowing Roar and Distracting Ash
-- are fixed single schedules in C++.
local function setupGroundPhase(creature, guid)
    local st = nightbaneState[guid]
    if st == nil then
        return
    end
    st.phase = PHASE_GROUND
    schedule(guid, "g_cleave", {1, 15000}, function() onCleave(creature, guid) end)
    schedule(guid, "g_tailsweep", {4000, 23000}, function() onTailSweep(creature, guid) end)
    schedule(guid, "g_roar", 48000, function() onBellowingRoar(creature, guid) end)
    schedule(guid, "g_charredearth", {12000, 18000}, function() onCharredEarth(creature, guid) end)
    schedule(guid, "g_smoldering", {26000, 30000}, function() onSmolderingBreath(creature, guid) end)
    schedule(guid, "g_ash", 82000, function() onDistractingAsh(creature, guid) end)
end

local function onRainOfBones(creature, guid)
    if nightbaneState[guid] == nil then
        return
    end
    -- C++ EVENT_RAIN_OF_BONES: ResetThreatList() before the random-target
    -- cast (bridged lua_creature_events.go); SetFacingToObject has no
    -- direct bridge — facing computed with math.atan2 (the Unit::
    -- GetAbsoluteAngle twin, zuljin precedent) then SetFacingTo.
    creature:ClearThreatList()
    local target = randomAlivePlayer(creature)
    if target ~= nil then
        local dx = target:GetX() - creature:GetX()
        local dy = target:GetY() - creature:GetY()
        local face = math.atan2(dy, dx)
        if face < 0 then face = face + 2 * math.pi end
        creature:SetFacingTo(face)
        -- C++ SetFacingToObject arm ported above; the AuraScript periodic
        -- skeleton summon has no AuraScript model.
        creature:CastSpell(target, SPELL_RAIN_OF_BONES)
    end
end

local function onEmoteBreath(creature, guid)
    if nightbaneState[guid] == nil then
        return
    end
    creature:Talk(EMOTE_BREATH)
    schedule(guid, "f_rainofbones", 3000, function() onRainOfBones(creature, guid) end)
end

local function onSmokingBlast(creature, guid)
    local target = randomAlivePlayer(creature)
    if target ~= nil then
        creature:CastSpell(target, SPELL_SMOKING_BLAST)
    end
    schedule(guid, "f_smokingblast", 1400, function() onSmokingBlast(creature, guid) end)
end

local function onSmokingBlastT(creature, guid)
    local target = randomAlivePlayer(creature)
    if target ~= nil then
        creature:CastSpell(target, SPELL_SMOKING_BLAST_T)
    end
    schedule(guid, "f_smokingblastt", {5000, 7000}, function() onSmokingBlastT(creature, guid) end)
end

local function onLanded(creature, guid)
    -- C++ EVENT_LANDED: SetupGroundPhase + SetReactState(AGGRESSIVE).
    setupGroundPhase(creature, guid)
end

local function onEndPhaseTwo(creature, guid)
    if nightbaneState[guid] == nil then
        return
    end
    -- C++ MovementInform POINT_PHASE_TWO_LANDING arrival
    -- (boss_nightbane.cpp:228):
    -- me->HandleEmoteCommand(EMOTE_ONESHOT_LAND) (SharedDefines.h:2007,
    -- value 293), before the 3s EVENT_LANDED schedule.
    creature:PerformEmote(293)
    -- C++ MoveAlongSplineChain(POINT_PHASE_TWO_LANDING, ...) arrival —
    -- 5s approximates the descent with no movement model.
    schedule(guid, "f_landed", 5000, function() onLanded(creature, guid) end)
end

local function onLand(creature, guid)
    if nightbaneState[guid] == nil then
        return
    end
    creature:Talk(YELL_LAND_PHASE)
    -- C++ MoveAlongSplineChain(POINT_PHASE_TWO_END, SPLINE_CHAIN_PHASE_TWO)
    -- arrival — 5s approximates the return flight with no movement model.
    schedule(guid, "f_endphasetwo", 5000, function() onEndPhaseTwo(creature, guid) end)
end

local function onPreLand(creature, guid)
    if nightbaneState[guid] == nil then
        return
    end
    cancelPrefix(guid, "f_")
    schedule(guid, "f_land", 2000, function() onLand(creature, guid) end)
end

local function onFlyArrived(creature, guid)
    -- C++ POINT_MOTION_TYPE POINT_PHASE_TWO_FLY arrival arm.
    if nightbaneState[guid] == nil then
        return
    end
    schedule(guid, "f_preland", 33000, function() onPreLand(creature, guid) end)
    schedule(guid, "f_emotebreath", 2000, function() onEmoteBreath(creature, guid) end)
    schedule(guid, "f_smokingblastt", 21000, function() onSmokingBlastT(creature, guid) end)
    schedule(guid, "f_smokingblast", 17000, function() onSmokingBlast(creature, guid) end)
end

local function startPhaseFly(creature, guid)
    local st = nightbaneState[guid]
    if st == nil then
        return
    end
    -- C++ StartPhaseFly: ++_flyCount, Talk(YELL_FLY_PHASE), cancel the
    -- ground group, interrupt non-melee casts, liftoff emote, gravity on,
    -- react passive, AttackStop — the liftoff emote ports via
    -- creature:PerformEmote (below); the other four have no bridge.
    st.flyCount = st.flyCount + 1
    st.phase = PHASE_FLY
    creature:Talk(YELL_FLY_PHASE)
    cancelPrefix(guid, "g_")
    -- C++ StartPhaseFly (boss_nightbane.cpp:258):
    -- me->HandleEmoteCommand(EMOTE_ONESHOT_LIFTOFF) (SharedDefines.h:2003,
    -- value 254), between the interrupt and the gravity legs.
    creature:PerformEmote(254)
    -- The MovePoint to the nearest pre-fly/fly position has no movement
    -- model; the arrival fires 1ms later exactly like C++'s
    -- EVENT_PRE_FLY_END -> MovePoint(POINT_PHASE_TWO_FLY) chain.
    schedule(guid, "f_preflyend", 1, function() onFlyArrived(creature, guid) end)
end

local function nightbaneInitState(guid)
    -- C++ ctor / Reset: _flyCount = 0, phase = PHASE_GROUND.
    nightbaneState[guid] = { phase = PHASE_GROUND, flyCount = 0 }
end

local function nightbaneEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    nightbaneInitState(guid)
    -- The intro (ACTION_SUMMON spline chain, IMMUNE_TO_PC, DoZoneInCombat)
    -- is skipped — no movement bridge (stale 'no gameobject-gossip
    -- bridge' retracted — RegisterGameObjectGossipEvent bridged,
    -- engine/world/gameobject_hooks.go) — so combat
    -- starts directly at the landing state: Talk + SetupGroundPhase, like
    -- C++ JustEngagedWith.
    creature:Talk(YELL_AGGRO)
    setupGroundPhase(creature, guid)
end

local function nightbaneLeaveCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    nightbaneInitState(guid)
end

local function nightbaneDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    -- C++ JustDied: HandleTerraceDoors(true) — no instance-script model.
end

-- C++ boss_nightbane.cpp:142-145: JustReachedHome -> _DespawnAtEvade()
-- (BossAI despawn with respawn delay + instance SetBossState(FAIL); the
-- instance leg has no bridge). Go fires event 24 at the same home-arrival
-- point; creature:Despawn() respawns DB-spawned creatures on their normal
-- spawntimesecs (opera precedent).
local function nightbaneReachHome(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    nightbaneInitState(guid)
    creature:Despawn()
end

local function nightbaneReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    nightbaneInitState(guid)
end

local function nightbaneDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local st = nightbaneState[guid]
    if st == nil then
        st = { phase = PHASE_GROUND, flyCount = 0 }
        nightbaneState[guid] = st
    end
    local health, maxHealth = creature:GetHealth(), creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local newDamage = damage
    if st.phase == PHASE_FLY then
        -- C++ DamageTaken PHASE_FLY arm: the boss cannot die mid-air
        -- (no health guard in C++; at exactly 1 HP a lethal hit becomes
        -- 0 damage, damage = health - 1).
        if damage >= health then
            newDamage = health - 1
        end
        return false, newDamage
    end
    -- C++ uses HealthBelowPct (pre-damage health), NOT the damaged variant;
    -- the integer floor form matches CalculatePct exactly.
    if (st.flyCount == 0 and health < math.floor(maxHealth * 75 / 100))
            or (st.flyCount == 1 and health < math.floor(maxHealth * 50 / 100))
            or (st.flyCount == 2 and health < math.floor(maxHealth * 25 / 100)) then
        startPhaseFly(creature, guid)
    end
    return false, newDamage
end

RegisterCreatureEvent(ENTRY_NIGHTBANE, 1, nightbaneEnterCombat)
RegisterCreatureEvent(ENTRY_NIGHTBANE, 2, nightbaneLeaveCombat)
RegisterCreatureEvent(ENTRY_NIGHTBANE, 4, nightbaneDied)
RegisterCreatureEvent(ENTRY_NIGHTBANE, 9, nightbaneDamageTaken)
RegisterCreatureEvent(ENTRY_NIGHTBANE, 23, nightbaneReset)
RegisterCreatureEvent(ENTRY_NIGHTBANE, 24, nightbaneReachHome)
