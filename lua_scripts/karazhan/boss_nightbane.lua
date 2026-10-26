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
-- port via creature:PerformEmote (254/293), and the react-state flips
-- port via the bridged creature:SetReactState (startPhaseFly ->
-- REACT_PASSIVE, onLanded -> REACT_AGGRESSIVE) — suppressing engine
-- melee during the fly-phase timer chain exactly like C++ (no melee
-- while airborne); the gravity flips port via the bridged
-- creature:SetDisableGravity (liftoff on, touchdown off), while
-- AttackStop and the interrupt-casts leg (both ported at
-- startPhaseFly below) split. GO gossip dispatch IS bridged
-- (RegisterGameObjectGossipEvent), but the go_blackened_urn gossip arm
-- (entry 194092) stays out: its boss-state gate (DATA_NIGHTBANE DONE /
-- IN_PROGRESS) has no instance bridge, its effect is a cross-creature
-- DoAction(ACTION_SUMMON) with no bridge, and the intro machine it
-- triggers (EMOTE_SUMMON, IMMUNE_TO_PC until landing, DoZoneInCombat)
-- is unported — so the port starts directly at the landing combat state.
-- Rain of Bones (37098) is cast on the random target only — its
-- AuraScript periodic skeleton-summon arm (SPELL_SUMMON_SKELETON 30170,
-- every 5th tick) has no bridge; AuraScript check handlers are a standing
-- unmodeled area. The EVENT_RAIN_OF_BONES ResetThreatList arm IS
-- ported (:205 — ResetAllThreat bridged). Tail
-- Sweep's HasInArc(PI) behind-target gate is PORTED via the inArc twin
-- (GetX/GetY/GetO are bridged). No UNIT_STATE_CASTING model, so timers
-- fire unconditionally.

local ENTRY_NIGHTBANE = 17225
local GO_MASTERS_TERRACE_DOOR = 184274 -- karazhan.h
local GO_MASTERS_TERRACE_DOOR2 = 184280
local GO_BLACKENED_URN = 194092

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

-- C++ SelectTarget(Random, 0, 0.0f, true) (boss_nightbane.cpp:279/:288/
-- :326/:337/:342/:347: charred earth, distracting ash, rain of bones,
-- smoking blast x2, tail sweep): the pool is the threat list, not the
-- instance players; dist 0.0f = no range check.
local function randomAlivePlayer(creature)
    local candidates = {}
    for _, unit in ipairs(creature:GetAITargets()) do
        if unit:GetObjectType() == "Player" and not unit:IsDead() then
            candidates[#candidates + 1] = unit
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
    -- C++ !me->HasInArc(M_PI, target) (boss_nightbane.cpp:348): tail sweep
    -- only hits behind the boss (the atan2 inArc twin was retired
    -- 2026-10-24 for the live creature:IsInBack bridge, twin-exact:
    -- !hasInArc(2*pi - pi) = !HasInArc(pi)); the 20-30s re-arm is
    -- unconditional (:350).
    if target ~= nil and creature:IsInBack(target) then
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
    creature:ResetAllThreat()
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
    -- C++ EVENT_LANDED (boss_nightbane.cpp:310-313): SetupGroundPhase()
    -- then SetReactState(REACT_AGGRESSIVE) — C++ arm order kept.
    setupGroundPhase(creature, guid)
    creature:SetReactState(2) -- REACT_AGGRESSIVE
end

local function onEndPhaseTwo(creature, guid)
    if nightbaneState[guid] == nil then
        return
    end
    -- C++ MovementInform POINT_PHASE_TWO_LANDING arrival
    -- (boss_nightbane.cpp:224-229): SetPhase(PHASE_GROUND),
    -- me->SetDisableGravity(false) (:225), then
    -- me->HandleEmoteCommand(EMOTE_ONESHOT_LAND) (SharedDefines.h:2007,
    -- value 293), before the 3s EVENT_LANDED schedule.
    creature:SetDisableGravity(false) -- C++ :225, before the land emote
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
    -- C++ EVENT_LAND (boss_nightbane.cpp:304-308): Talk(YELL_LAND_PHASE),
    -- me->SetDisableGravity(true) (:306) — already true since
    -- startPhaseFly, idempotent but C++-order-exact — then the
    -- MoveAlongSplineChain(POINT_PHASE_TWO_END) whose arrival is
    -- approximated below.
    creature:SetDisableGravity(true)
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
    -- creature:PerformEmote (below); the gravity leg ports via the
    -- creature:SetDisableGravity bridge; only the MovePoint has no bridge.
    st.flyCount = st.flyCount + 1
    st.phase = PHASE_FLY
    creature:Talk(YELL_FLY_PHASE)
    cancelPrefix(guid, "g_")
    -- C++ StartPhaseFly (boss_nightbane.cpp:257):
    -- me->InterruptNonMeleeSpells(false), between the ground-group
    -- cancel and the liftoff emote.
    creature:InterruptNonMeleeSpells(false)
    -- C++ StartPhaseFly (boss_nightbane.cpp:258):
    -- me->HandleEmoteCommand(EMOTE_ONESHOT_LIFTOFF) (SharedDefines.h:2003,
    -- value 254), between the interrupt and the gravity legs.
    creature:PerformEmote(254)
    -- C++ StartPhaseFly (boss_nightbane.cpp:252-270): ++_flyCount,
    -- Talk(YELL_FLY_PHASE), cancel ground group, interrupt casts,
    -- liftoff emote, gravity on, SetReactState(REACT_PASSIVE) (:260),
    -- AttackStop (:261), MovePoint — C++ arm order kept for the modeled
    -- legs (MovePoint has no bridge; the intro SPLINE_CHAIN still has
    -- none). PASSIVE while
    -- airborne: the engine's isCreaturePassive gate suppresses melee
    -- during the fly-phase timer chain, matching C++ (no melee mid-air).
    creature:SetDisableGravity(true) -- C++ :258, between emote and react
    creature:SetReactState(0) -- REACT_PASSIVE
    -- C++ boss_nightbane.cpp:261: me->AttackStop()
    creature:AttackStop()
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
    -- C++ EnterEvadeMode (boss_nightbane.cpp:138): SetDisableGravity(true);
    -- ev2 is Go's EnterEvadeMode/OnLeaveCombat hook (lua_creature_events.go).
    creature:SetDisableGravity(true)
end

local function nightbaneDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    -- C++ JustDied: HandleTerraceDoors(true) = both terrace doors open
    -- (GO_STATE_ACTIVE) via the netherspite FindNearestGameObject precedent.
    -- The intro close (DoAction ACTION_SUMMON) stays out: no inbound
    -- DoAction dispatch bridge.
    for _, entry in ipairs({ GO_MASTERS_TERRACE_DOOR, GO_MASTERS_TERRACE_DOOR2 }) do
        local door = creature:FindNearestGameObject(entry, 100.0)
        if door then door:SetGoState(0) end
    end
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
    -- C++ boss_nightbane::Reset (:126-133): _Reset(), _flyCount = 0,
    -- me->SetDisableGravity(true) (:130) — bridged (lua_creature_events.go,
    -- PORTED 2026-10-22) + HandleTerraceDoors(true) (both doors open) +
    -- the blackened urn's GO_FLAG_IN_USE cleared unconditionally (no boss
    -- state gate in the C++ Reset body; GO gossip itself is bridged).
    creature:SetDisableGravity(true)
    for _, entry in ipairs({ GO_MASTERS_TERRACE_DOOR, GO_MASTERS_TERRACE_DOOR2 }) do
        local door = creature:FindNearestGameObject(entry, 100.0)
        if door then door:SetGoState(0) end
    end
    local urn = creature:FindNearestGameObject(GO_BLACKENED_URN, 100.0)
    if urn then urn:RemoveFlag(9, 1) end -- GAMEOBJECT_FLAGS, GO_FLAG_IN_USE
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
