-- The Four Horsemen (Naxxramas, Military Quarter) — Lua port of
-- src/server/scripts/Northrend/Naxxramas/boss_four_horsemen.cpp
-- (boss_four_horsemen_baron / _thane / _lady / _sir, all on
-- boss_four_horsemen_baseAI (BossAI); AddSC_boss_four_horsemen
-- also registers spell_four_horsemen_mark (AuraScript) and
-- spell_four_horsemen_consumption (SpellScript), unmodeled).
-- Entries: 16064 Thane Korth'azz, 16065 Lady Blaumeux,
-- 30549 Baron Rivendare, 16063 Sir Zeliek (naxxramas.h
-- NAXCreaturesIds; 25-man difficulty entries 30603 / 30601 /
-- 30600 / 30602 are NOT registered, base-entry-only
-- convention like anubrekhan/maexxna).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 15 OnSpellHitTarget, 23 OnReset.
-- Timers via CreateLuaEvent; melee is engine-driven in Go,
-- like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(target, spell) = DoCast,
-- creature:CastSpell(creature, spell) = DoCastSelf
-- (DoCastAOE is lowered to self-cast, selin/mr_smite
-- precedent); the trailing true is the triggered port.
-- Ported arms (C++-exact for all modeled arms):
-- Reset (event 23): SetReactState(2) AGGRESSIVE +
-- SetCombatMovement(false) + SetCombatPulseDelay(0)
-- (C++ Reset order; ResetLootMode / summons.DespawnAll
-- have no bridge — see below); JustEngagedWith Talk
-- (SAY_AGGRO 0, once per encounter via the boss-state
-- gate) + BeginEncounter; DoAction BEGIN_MOVEMENT (1) +
-- MovementInform corner chain + CheckIsMovementFinished
-- + DoAction BEGIN_FIGHTING (2) ported tick 559 (see
-- below) — the four run to their corners before the
-- per-horseman BeginFighting schedules arm;
-- Baron BeginFighting: SetCombatMovement(true) +
-- SetReactState(2) + EVENT_BERSERK 10min one-shot
-- (DoCastAOE BERSERK 26662, triggered, self-cast) +
-- EVENT_MARK 24s (DoCastAOE BARON_MARK 28834, triggered,
-- self-cast, 12s repeat) + EVENT_UNHOLYSHADOW
-- randtime(3s,7s) (DoCastVictim UNHOLY_SHADOW 28882,
-- randtime(10s,30s) repeat); SpellHitTarget (event 15)
-- UNHOLY_SHADOW hit -> Talk(SAY_SPECIAL 2);
-- Thane BeginFighting: SetCombatMovement(true) +
-- SetReactState(2) + BERSERK 10min + MARK 24s
-- (THANE_MARK 28832, triggered, self-cast, 12s repeat) +
-- METEOR randtime(10s,25s) (random player within 20yd,
-- DoCast METEOR 28884, randtime(13s,17s) repeat,
-- _shouldSay re-armed on cast); SpellHitTarget METEOR
-- hit + _shouldSay -> Talk(SAY_SPECIAL 2), latch cleared;
-- Lady BeginFighting: BERSERK 10min + MARK 24s
-- (LADY_MARK 28833, triggered, self-cast, 15s repeat) +
-- VOIDZONE randtime(5s,10s) (random player within 45yd,
-- DoCast VOID_ZONE 28863, triggered, + Talk(SAY_SPECIAL 2),
-- randtime(12s,18s) repeat) + 2s filler pump (nearest
-- target within 45yd -> DoCast SHADOW_BOLT 57374, else
-- DoCastAOE UNYIELDING_PAIN 57381 self-cast + Talk
-- (EMOTE_RAGECAST 7) — C++ casts this every UpdateAI tick,
-- the 2s pump is the bronjahm filler precedent, the
-- engine cast time being the real gate in C++);
-- Sir BeginFighting: BERSERK 10min + MARK 24s
-- (SIR_MARK 28835, triggered, self-cast, 15s repeat) +
-- HOLYWRATH randtime(13s,18s) (nearest player within
-- 45yd, DoCast HOLY_WRATH 28883, triggered, _shouldSay
-- re-armed on cast, randtime(10s,18s) repeat) + 2s filler
-- pump (nearest target within 45yd -> DoCast HOLY_BOLT
-- 57376, else DoCastAOE CONDEMNATION 57377 self-cast +
-- Talk(EMOTE_RAGECAST 7)); SpellHitTarget HOLY_WRATH
-- hit + _shouldSay -> Talk(SAY_SPECIAL 2), latch cleared.
-- KilledUnit Talk(SAY_SLAY 3) player-gated (event 3,
-- commanders precedent); JustDied Talk(SAY_DEATH 4).
-- EncounterCleared credit cast PORTED tick 246: DoCastAOE
-- (ENCOUNTER_CREDIT 59450, triggered) self-cast when the fourth
-- horseman dies — the all-four-done branch in the died handler is
-- the C++-exact trigger point; the spell's effect is data-side.
-- CASTING gates, partial, PORTED 2026-11-04: each derived
-- _UpdateAI's HasUnitState(UNIT_STATE_CASTING) check sits AFTER
-- the event loop (baron :441, thane :517, lady :592, sir :662),
-- so berserk/mark/unholy/meteor/void/wrath event arms fire
-- UNGATED and stay gateless; the check gates only the post-loop
-- arms — baron/thane melee (engine-driven, nothing to port) and
-- the lady/sir 2s filler pumps, which now defer 500ms while
-- casting (mal_ganis idiom).
-- All timers cancelled on 2/4/23 (gargolmar precedent).
-- Distance filters use creature:GetDistance (amanitar /
-- bpc precedent); the 45yd/20yd bounds are C++-exact.
-- Unmodeled (no bridges — documented, not wired):
-- BeginEncounter's CheckRequiredBosses gate, setActive /
-- SetFarVisible legs; BeginFighting AttackStart legs:
-- baron/thane threat-nonempty AttackStart(current victim)
-- ported 2026-10-24 via EngageWithTarget(GetVictim()) in
-- C++ order; threat-empty nearest-player + AddThreat leg
-- stays out (no player-search bridge on an empty threat
-- list; ResetEncounter unbridged); lady/sir have no
-- BeginFighting AttackStart leg in C++ (their threat-empty
-- EnterEvadeMode legs stay unbridged);
-- JustDied: the one-shot BeginEncounter path + the
-- CheckIsMovementFinished call (another horseman alive)
-- ported tick 559; the death-time latches ARE ported:
-- <which>DeathMS via GetGameTimeMS, read by the Go
-- 7600/7601 credit check, cleared by Reset; the
-- EncounterCleared credit cast itself IS ported, see
-- above; SpellHitTarget on lady voidzone
-- (C++ has no SpellHitTarget arm for lady — only the
-- direct Talk on cast, ported above); the mark
-- AuraScript stack-damage table (SPELL_MARK_DAMAGE
-- 28836) and consumption SpellScript (no SpellScript
-- bridge); Reset ResetLootMode + summons.DespawnAll
-- (no bridges); ctor respawn-time leg (a dead horseman respawns
-- in 10s while the encounter is not DONE — instance-state-gated,
-- no bridge); EnterEvadeMode -> ResetEncounter (instance
-- NOT_STARTED + despawn all — no bridge); C++ Talk(SAY_AGGRO)
-- fires once for the whole encounter (instance-gated) —
-- ported tick 559 via the boss-state gate in enterCombat.

local ENTRY_THANE = 16064
local ENTRY_LADY = 16065
local ENTRY_BARON = 30549
local ENTRY_SIR = 16063

local SAY_AGGRO = 0
local SAY_SPECIAL = 2
local SAY_SLAY = 3
local SAY_DEATH = 4
local EMOTE_RAGECAST = 7

local SPELL_BERSERK = 26662
local SPELL_BARON_MARK = 28834
local SPELL_THANE_MARK = 28832
local SPELL_LADY_MARK = 28833
local SPELL_SIR_MARK = 28835
local SPELL_UNHOLY_SHADOW = 28882
local SPELL_METEOR = 28884
local SPELL_VOID_ZONE = 28863
local SPELL_SHADOW_BOLT = 57374
local SPELL_UNYIELDING_PAIN = 57381
local SPELL_HOLY_WRATH = 28883
local SPELL_HOLY_BOLT = 57376
local SPELL_CONDEMNATION = 57377
local SPELL_ENCOUNTER_CREDIT = 59450

-- boss_four_horsemen.cpp:78-80 Actions; :96-100 HorsemenData
-- (DATA_HORSEMEN_IS_TIMED_KILL = NAXData 2, naxxramas.h:52,
-- so DATA_MOVEMENT_FINISHED = 3). POINT_MOTION_TYPE = 8
-- (runtime.go:389-397, the ev28 type arg).
local ACTION_BEGIN_MOVEMENT = 1
local ACTION_BEGIN_FIGHTING = 2
local DATA_MOVEMENT_FINISHED = 3
local POINT_MOTION_TYPE = 8

-- C++ :117-120 corner paths, one per horseman (3 waypoints each).
local PATHS = {
    baron = {{2552.427, -2969.737, 241.3021}, {2566.759, -2972.535, 241.3217}, {2584.32, -2971.96, 241.3489}},
    thane = {{2540.095, -2983.192, 241.3344}, {2546.005, -2999.826, 241.3665}, {2542.697, -3014.055, 241.3371}},
    lady = {{2507.94, -2961.444, 242.4557}, {2488.763, -2960.007, 241.2757}, {2468.26, -2947.499, 241.2753}},
    sir = {{2533.141, -2922.14, 241.2764}, {2525.254, -2905.907, 241.2761}, {2517.636, -2897.253, 241.2758}},
}
local HORSEMEN = {
    { entry = ENTRY_BARON, which = "baron" },
    { entry = ENTRY_THANE, which = "thane" },
    { entry = ENTRY_LADY, which = "lady" },
    { entry = ENTRY_SIR, which = "sir" },
}

local timers = {}
local shouldSay = {}
-- C++ baseAI members: _nextMovement (banked arrival id) and
-- _ourMovementFinished (BEGIN_FIGHTING already received), per GUID.
-- _myMovementFinished itself lives in the engine ScriptData store
-- (SetData/GetData twin) so other horsemen can read it cross-AI.
local nextMovement = {}
local ourMovementFinished = {}

local function cancelTimers(guid)
    local per = timers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        timers[guid] = nil
    end
    shouldSay[guid] = nil
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

-- C++ SelectTarget dist gate is IsWithinCombatRange (Unit.cpp:
-- 583-598: 3D dist < nominal + both combat reaches; player reach is
-- DEFAULT_PLAYER_COMBAT_REACH 1.5, ObjectDefines.h:40 — player
-- scale is 1.0 in practice, so the constant is C++-exact here).
local PLAYER_COMBAT_REACH = 1.5

local function randomPlayerInRange(creature, maxDist)
    local candidates = {}
    local bound = maxDist + creature:GetCombatReach() + PLAYER_COMBAT_REACH
    for _, target in ipairs(creature:GetAITargets()) do
        if target:GetObjectType() == "Player"
                and not target:IsDead()
                and creature:GetDistance(target) < bound then
            candidates[#candidates + 1] = target
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

local function nearestInRange(creature, maxDist, playerOnly)
    local best = nil
    local bestDist = nil
    local casterReach = creature:GetCombatReach()
    for _, target in ipairs(creature:GetAITargets()) do
        if not target:IsDead()
                and (not playerOnly or target:GetObjectType() == "Player") then
            local targetReach = PLAYER_COMBAT_REACH
            if target:GetObjectType() ~= "Player" then
                targetReach = target:GetCombatReach()
            end
            local d = creature:GetDistance(target)
            if d < maxDist + casterReach + targetReach
                    and (bestDist == nil or d < bestDist) then
                best = target
                bestDist = d
            end
        end
    end
    return best
end

local function berserkTick(creature, guid)
    creature:CastSpell(creature, SPELL_BERSERK, true)
end

local function markTick(creature, guid, spell, repeatMs)
    creature:CastSpell(creature, spell, true)
    schedule(guid, "mark", repeatMs, function()
        markTick(creature, guid, spell, repeatMs)
    end)
end

local function baronUnholyTick(creature, guid)
    local victim = creature:GetVictim()
    if victim ~= nil then
        creature:CastSpell(victim, SPELL_UNHOLY_SHADOW)
    end
    schedule(guid, "unholy", math.random(10000, 30000), function()
        baronUnholyTick(creature, guid)
    end)
end

local function thaneMeteorTick(creature, guid)
    local target = randomPlayerInRange(creature, 20)
    if target ~= nil then
        creature:CastSpell(target, SPELL_METEOR)
        shouldSay[guid] = true
    end
    schedule(guid, "meteor", math.random(13000, 17000), function()
        thaneMeteorTick(creature, guid)
    end)
end

local function ladyVoidTick(creature, guid)
    local target = randomPlayerInRange(creature, 45)
    if target ~= nil then
        creature:CastSpell(target, SPELL_VOID_ZONE, true)
        creature:Talk(SAY_SPECIAL)
    end
    schedule(guid, "void", math.random(12000, 18000), function()
        ladyVoidTick(creature, guid)
    end)
end

local function ladyBoltTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bolt", 500, function()
            ladyBoltTick(creature, guid)
        end)
        return
    end
    local target = nearestInRange(creature, 45, false)
    if target ~= nil then
        creature:CastSpell(target, SPELL_SHADOW_BOLT)
    else
        creature:CastSpell(creature, SPELL_UNYIELDING_PAIN)
        creature:Talk(EMOTE_RAGECAST)
    end
    schedule(guid, "bolt", 2000, function()
        ladyBoltTick(creature, guid)
    end)
end

local function sirWrathTick(creature, guid)
    local target = nearestInRange(creature, 45, true)
    if target ~= nil then
        creature:CastSpell(target, SPELL_HOLY_WRATH, true)
        shouldSay[guid] = true
    end
    schedule(guid, "wrath", math.random(10000, 18000), function()
        sirWrathTick(creature, guid)
    end)
end

local function sirBoltTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bolt", 500, function()
            sirBoltTick(creature, guid)
        end)
        return
    end
    local target = nearestInRange(creature, 45, false)
    if target ~= nil then
        creature:CastSpell(target, SPELL_HOLY_BOLT)
    else
        creature:CastSpell(creature, SPELL_CONDEMNATION)
        creature:Talk(EMOTE_RAGECAST)
    end
    schedule(guid, "bolt", 2000, function()
        sirBoltTick(creature, guid)
    end)
end

local function beginFighting(creature, guid, which)
    creature:SetCombatPulseDelay(5)
    if which == "baron" or which == "thane" then
        creature:SetCombatMovement(true)
        creature:SetReactState(2)
        local victim = creature:GetVictim()
        if victim ~= nil then
            creature:EngageWithTarget(victim)
        end
    end
    schedule(guid, "berserk", 600000, function()
        berserkTick(creature, guid)
    end)
    if which == "baron" then
        schedule(guid, "mark", 24000, function()
            markTick(creature, guid, SPELL_BARON_MARK, 12000)
        end)
        schedule(guid, "unholy", math.random(3000, 7000), function()
            baronUnholyTick(creature, guid)
        end)
    elseif which == "thane" then
        schedule(guid, "mark", 24000, function()
            markTick(creature, guid, SPELL_THANE_MARK, 12000)
        end)
        schedule(guid, "meteor", math.random(10000, 25000), function()
            thaneMeteorTick(creature, guid)
        end)
    elseif which == "lady" then
        schedule(guid, "mark", 24000, function()
            markTick(creature, guid, SPELL_LADY_MARK, 15000)
        end)
        schedule(guid, "void", math.random(5000, 10000), function()
            ladyVoidTick(creature, guid)
        end)
        schedule(guid, "bolt", 2000, function()
            ladyBoltTick(creature, guid)
        end)
    elseif which == "sir" then
        schedule(guid, "mark", 24000, function()
            markTick(creature, guid, SPELL_SIR_MARK, 15000)
        end)
        schedule(guid, "wrath", math.random(13000, 18000), function()
            sirWrathTick(creature, guid)
        end)
        schedule(guid, "bolt", 2000, function()
            sirBoltTick(creature, guid)
        end)
    end
end

local BOSS_HORSEMEN = 12 -- naxxramas.h:42 (NAXEncounter)
local BOSS_STATE_NOT_STARTED = 0
local BOSS_STATE_IN_PROGRESS = 1
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local GAMEOBJECT_FLAGS = 9
local GO_FLAG_NOT_SELECTABLE = 16
local GO_NAXX_PORTAL_MILITARY = 181578 -- naxxramas.h:168
local GO_HORSEMEN_CHEST = 181366 -- naxxramas.h:117
local GO_HORSEMEN_CHEST_HERO = 193426 -- naxxramas.h:116

-- C++ CheckIsMovementFinished (:197-214): every alive horseman must
-- report GetData(DATA_MOVEMENT_FINISHED); a missing handle resets the
-- encounter (ResetEncounter leg — instance despawn, no bridge, so a
-- missing handle just aborts the check here). When all four finished,
-- each gets DoAction(ACTION_BEGIN_FIGHTING).
local function checkIsMovementFinished(creature)
    local handles = {}
    for _, h in ipairs(HORSEMEN) do
        local boss = creature:FindNearestCreature(h.entry, 500)
        if boss == nil then
            return
        end
        handles[#handles + 1] = boss
    end
    for _, boss in ipairs(handles) do
        if boss:IsAlive() and boss:GetData(DATA_MOVEMENT_FINISHED) ~= 1 then
            return
        end
    end
    for _, boss in ipairs(handles) do
        boss:DoAction(ACTION_BEGIN_FIGHTING)
    end
end

-- C++ BeginEncounter (:216-260): boss state IN_PROGRESS (the
-- CheckRequiredBosses gate has no bridge and stays out), all four
-- PASSIVE + AttackStop + AddThreat 0 to every alive non-GM player in
-- range, then DoAction(ACTION_BEGIN_MOVEMENT) each. setActive /
-- SetFarVisible have no bridge.
local function beginEncounter(creature)
    if creature:GetBossState(BOSS_HORSEMEN) == BOSS_STATE_IN_PROGRESS then
        return
    end
    creature:SetBossState(BOSS_HORSEMEN, BOSS_STATE_IN_PROGRESS)
    local players = creature:GetPlayersInRange(500)
    for _, h in ipairs(HORSEMEN) do
        local boss = creature:FindNearestCreature(h.entry, 500)
        if boss ~= nil and boss:IsAlive() then
            boss:SetReactState(0) -- REACT_PASSIVE
            boss:AttackStop()
            for _, player in ipairs(players) do
                if not player:IsGM() and not player:IsDead() then
                    boss:AddThreat(player, 0)
                end
            end
            boss:DoAction(ACTION_BEGIN_MOVEMENT)
        end
    end
end

local function makeHandlers(which)
    local function enterCombat(event, creature)
        -- C++ JustEngagedWith (:284-289): another horseman already
        -- started the encounter (IN_PROGRESS/DONE) -> return without
        -- Talk; else Talk(SAY_AGGRO) + BeginEncounter.
        if creature:GetBossState(BOSS_HORSEMEN) == BOSS_STATE_IN_PROGRESS
                or creature:GetBossState(BOSS_HORSEMEN) == BOSS_STATE_DONE then
            return
        end
        local guid = creature:GetGUIDLow()
        shouldSay[guid] = (which == "thane" or which == "sir")
        creature:Talk(SAY_AGGRO)
        beginEncounter(creature)
    end
    local function leaveCombat(event, creature)
        local guid = creature:GetGUIDLow()
        cancelTimers(guid)
        -- C++ evade despawns all four (ResetEncounter) and the respawn
        -- brings fresh AI state; Lua has no despawn leg, so the
        -- movement latches clear here instead.
        nextMovement[guid] = nil
        ourMovementFinished[guid] = nil
    end
    local function targetDied(event, creature, victim)
        if victim ~= nil and victim:GetObjectType() == "Player" then
            creature:Talk(SAY_SLAY)
        end
    end
    local function died(event, creature, killer)
        local guid = creature:GetGUIDLow()
        cancelTimers(guid)
        -- C++ JustDied (:319-323): killed before the encounter started
        -- (one-shot) -> BeginEncounter and return, no Talk, no _timeDied.
        if creature:GetBossState(BOSS_HORSEMEN) ~= BOSS_STATE_IN_PROGRESS
                and creature:GetBossState(BOSS_HORSEMEN) ~= BOSS_STATE_DONE then
            beginEncounter(creature)
            return
        end
        creature:Talk(SAY_DEATH)
        -- C++ SetBossState(BOSS_HORSEMEN, DONE) legs
        -- (instance_naxxramas.cpp): when the fourth horseman dies,
        -- the chest flag clear (respawn leg stays out — no GO
        -- GetRespawnDelay bridge) + military portal flag clear +
        -- DONE flag for the instance pump's wing taunt.
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                -- C++ JustDied (boss_four_horsemen.cpp:328):
                -- _timeDied = GameTime::GetGameTimeMS(), set only on
                -- the IN_PROGRESS path (:321-323 — the NOT_STARTED
                -- one-shot path calls BeginEncounter and returns
                -- without banking); the Go 7600/7601 credit check
                -- reads the four <which>DeathMS stamps and requires
                -- max-min <= 15000ms (DATA_HORSEMEN_IS_TIMED_KILL
                -- :140-164, read by the instance script via the
                -- baron's GetData, instance_naxxramas.cpp:530-533).
                if creature:GetBossState(BOSS_HORSEMEN) == BOSS_STATE_IN_PROGRESS then
                    data[which .. "DeathMS"] = GetGameTimeMS()
                end
                data[which .. "Done"] = true
                if data.baronDone and data.thaneDone and data.ladyDone and data.sirDone and not data.horsemenDone then
                    data.horsemenDone = true
                    creature:SetBossState(BOSS_HORSEMEN, BOSS_STATE_DONE) -- EncounterCleared (boss_four_horsemen.cpp:279), before the credit cast below
                    local chest = creature:FindNearestGameObject(GO_HORSEMEN_CHEST, 1000)
                    if not chest then
                        chest = creature:FindNearestGameObject(GO_HORSEMEN_CHEST_HERO, 1000)
                    end
                    if chest then
                        chest:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
                    end
                    local portal = creature:FindNearestGameObject(GO_NAXX_PORTAL_MILITARY, 1000)
                    if portal then
                        portal:RemoveFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
                    end
                    -- C++ EncounterCleared: after SetBossState(DONE),
                    -- DoCastAOE(ENCOUNTER_CREDIT 59450, triggered).
                    creature:CastSpell(creature, SPELL_ENCOUNTER_CREDIT, true)
                end
                if not (data.baronDone and data.thaneDone and data.ladyDone and data.sirDone) then
                    -- C++ JustDied (:330-337): another horseman still
                    -- alive (possibly still running to its corner) ->
                    -- CheckIsMovementFinished.
                    checkIsMovementFinished(creature)
                end
            end
        end
    end
    local function reset(event, creature)
        local guid = creature:GetGUIDLow()
        cancelTimers(guid)
        -- C++ Reset (boss_four_horsemen.cpp:303): _myMovementFinished =
        -- false (ScriptData clear), _nextMovement = 0, _timeDied = 0,
        -- _ourMovementFinished = false.
        creature:SetData(DATA_MOVEMENT_FINISHED, 0)
        nextMovement[guid] = nil
        ourMovementFinished[guid] = nil
        -- C++ Reset (boss_four_horsemen.cpp:303): _timeDied = 0.
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                data[which .. "DeathMS"] = 0
            end
        end
        if creature:GetBossState(BOSS_HORSEMEN) ~= BOSS_STATE_DONE then
            creature:SetBossState(BOSS_HORSEMEN, BOSS_STATE_NOT_STARTED) -- BossAI::_Reset (ScriptedCreature.cpp:510), != DONE-gated
        end
        creature:SetReactState(2)
        creature:SetCombatMovement(false)
        creature:SetCombatPulseDelay(0)
    end
    local function doAction(event, creature, action)
        local guid = creature:GetGUIDLow()
        if action == ACTION_BEGIN_MOVEMENT then
            -- C++ DoAction (:173-176): MovePoint(1, _initialPath[0],
            -- true); MoveTo fires the ev28 POINT_MOTION_TYPE arrival
            -- (lua_creature_events.go:4867).
            local p = PATHS[which]
            creature:MoveTo(1, p[1][1], p[1][2], p[1][3])
        elseif action == ACTION_BEGIN_FIGHTING then
            -- C++ DoAction (:177-183): already-finished guard, then
            -- SetCombatPulseDelay(5) (inside beginFighting) + BeginFighting.
            if ourMovementFinished[guid] then
                return
            end
            beginFighting(creature, guid, which)
            ourMovementFinished[guid] = true
        end
    end
    local function movementInform(event, creature, motionType, id)
        if motionType ~= POINT_MOTION_TYPE then
            return
        end
        local guid = creature:GetGUIDLow()
        if id < 3 then
            -- C++ MovementInform (:336-347): id < 3 banks _nextMovement
            -- and the next UpdateAI tick issues MovePoint(id + 1,
            -- _initialPath[id]). Lua has no UpdateAI hook, so the next
            -- leg is re-issued after a 100ms pump (one server-tick delay).
            nextMovement[guid] = id
            schedule(guid, "nextmove", 100, function()
                local step = nextMovement[guid]
                nextMovement[guid] = nil
                if step ~= nil then
                    local p = PATHS[which]
                    creature:MoveTo(step + 1, p[step + 1][1], p[step + 1][2], p[step + 1][3])
                end
            end)
        else
            -- C++ :344-347: _myMovementFinished = true, then
            -- CheckIsMovementFinished. The flag lives in the engine
            -- ScriptData store (GetData/SetData twin) so the other
            -- horsemen's checks can read it cross-AI.
            creature:SetData(DATA_MOVEMENT_FINISHED, 1)
            checkIsMovementFinished(creature)
        end
    end
    return enterCombat, leaveCombat, targetDied, died, reset, doAction, movementInform
end

local function spellHitTarget(event, creature, target, spellId)
    local guid = creature:GetGUIDLow()
    if spellId == SPELL_UNHOLY_SHADOW then
        creature:Talk(SAY_SPECIAL)
    elseif spellId == SPELL_METEOR and shouldSay[guid] then
        creature:Talk(SAY_SPECIAL)
        shouldSay[guid] = false
    elseif spellId == SPELL_HOLY_WRATH and shouldSay[guid] then
        creature:Talk(SAY_SPECIAL)
        shouldSay[guid] = false
    end
end

local baronEnter, baronLeave, baronTargetDied, baronDied, baronReset,
    baronDoAction, baronMovementInform = makeHandlers("baron")
local thaneEnter, thaneLeave, thaneTargetDied, thaneDied, thaneReset,
    thaneDoAction, thaneMovementInform = makeHandlers("thane")
local ladyEnter, ladyLeave, ladyTargetDied, ladyDied, ladyReset,
    ladyDoAction, ladyMovementInform = makeHandlers("lady")
local sirEnter, sirLeave, sirTargetDied, sirDied, sirReset,
    sirDoAction, sirMovementInform = makeHandlers("sir")

RegisterCreatureEvent(ENTRY_BARON, 1, baronEnter)
RegisterCreatureEvent(ENTRY_BARON, 2, baronLeave)
RegisterCreatureEvent(ENTRY_BARON, 3, baronTargetDied)
RegisterCreatureEvent(ENTRY_BARON, 4, baronDied)
RegisterCreatureEvent(ENTRY_BARON, 15, spellHitTarget)
RegisterCreatureEvent(ENTRY_BARON, 23, baronReset)
RegisterCreatureEvent(ENTRY_BARON, 28, baronMovementInform)
RegisterCreatureEvent(ENTRY_BARON, 38, baronDoAction)

RegisterCreatureEvent(ENTRY_THANE, 1, thaneEnter)
RegisterCreatureEvent(ENTRY_THANE, 2, thaneLeave)
RegisterCreatureEvent(ENTRY_THANE, 3, thaneTargetDied)
RegisterCreatureEvent(ENTRY_THANE, 4, thaneDied)
RegisterCreatureEvent(ENTRY_THANE, 15, spellHitTarget)
RegisterCreatureEvent(ENTRY_THANE, 23, thaneReset)
RegisterCreatureEvent(ENTRY_THANE, 28, thaneMovementInform)
RegisterCreatureEvent(ENTRY_THANE, 38, thaneDoAction)

RegisterCreatureEvent(ENTRY_LADY, 1, ladyEnter)
RegisterCreatureEvent(ENTRY_LADY, 2, ladyLeave)
RegisterCreatureEvent(ENTRY_LADY, 3, ladyTargetDied)
RegisterCreatureEvent(ENTRY_LADY, 4, ladyDied)
RegisterCreatureEvent(ENTRY_LADY, 23, ladyReset)
RegisterCreatureEvent(ENTRY_LADY, 28, ladyMovementInform)
RegisterCreatureEvent(ENTRY_LADY, 38, ladyDoAction)

RegisterCreatureEvent(ENTRY_SIR, 1, sirEnter)
RegisterCreatureEvent(ENTRY_SIR, 2, sirLeave)
RegisterCreatureEvent(ENTRY_SIR, 3, sirTargetDied)
RegisterCreatureEvent(ENTRY_SIR, 4, sirDied)
RegisterCreatureEvent(ENTRY_SIR, 15, spellHitTarget)
RegisterCreatureEvent(ENTRY_SIR, 23, sirReset)
RegisterCreatureEvent(ENTRY_SIR, 28, sirMovementInform)
RegisterCreatureEvent(ENTRY_SIR, 38, sirDoAction)
