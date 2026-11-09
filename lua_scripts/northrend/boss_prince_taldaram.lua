-- Prince Taldaram (Ahn'kahet) — Lua port of
-- src/server/scripts/Northrend/AzjolNerub/Ahnkahet/boss_prince_taldaram.cpp
-- (boss_prince_taldaram). Ahn'kahet dungeon-script unit per
-- northrend_script_loader.cpp order (decl 33 / call 223, immediately
-- after AddSC_boss_elder_nadox(); next: boss_amanitar).
-- Entry: 29308 Prince Taldaram (ahnkahet.h NPC_PRINCE_TALDARAM —
-- kalecgos pass; the GetAhnKahetAI ScriptName binding is
-- instance-shimmed, the creature_template binding DB-side as usual).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 23 OnReset. Timers via CreateLuaEvent; melee is
-- engine-driven in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- C++ DoCast default is triggered=false (vaelastrasz convention):
-- creature:CastSpell(nil, spell) = DoCastVictim,
-- creature:CastSpell(creature, spell) = DoCastSelf (moroes precedent).
-- OnLeaveCombat(2)/OnDied(4)/OnReset(23) cancel the scheduler (gargolmar
-- precedent); BossAI _JustDied instance bookkeeping ports in
-- taldaramDied below (SetBossState + instanceData flag).
-- Ported arms (C++-exact for all modeled arms):
-- boss_prince_taldaram: JustEngagedWith Talk(SAY_AGGRO 2) (event 1);
-- EVENT_BLOODTHIRST DoCastSelf(Bloodthirst 55968), 10s init, 10s repeat;
-- EVENT_CONJURE_FLAME_SPHERES DoCast(GetVictim(), Conjure Flame Sphere
-- 55931), 5s init, 15s repeat (creature:GetVictim() — slad_ran viper
-- precedent; C++ casts on the victim with the "random target?" note, not
-- triggered); KilledUnit Talk(SAY_SLAY 3) player-gated (event 3,
-- victim:GetObjectType()=="Player" — nalorakk precedent); JustDied
-- Talk(SAY_DEATH 4) (event 4); KilledUnit embrace clear PORTED (event 3:
-- victim GUID == stored embrace target GUID -> clear, before Talk).
-- Ported vanish/feed machine (C++ :191-243): EVENT_VANISH (25-35s
-- init/repeat) gated on #GetAITargets() > 1 (GetThreatListSize twin),
-- pick = random alive threat-list player within 100yd (SelectTarget
-- Random,0,100,true twin; the stored player object replaces the
-- ObjectAccessor::GetUnit lookup), Talk(SAY_VANISH 6) + DoCastSelf
-- (VANISH 55964) + START_FEEDING in 2s; START_FEEDING removes VANISH,
-- casts Shadowstep 55966 + Embrace 55959, Talk(SAY_FEED 5), DONE_FEEDING
-- in 20s (target clear only — the damage counter persists across feeds
-- per C++); DamageTaken latch (event 9): accumulate while the embrace
-- target is alive, past 20000 / heroic 40000 -> clear target +
-- StopSpellCast (interrupts the live Embrace 55959 channel).
-- Deviations: C++ DelayEvents(500ms) on VANISH has no Lua twin
-- (independent CreateLuaEvent timers); the UpdateAI vanish-evade
-- check stays out — creature:EnterEvadeMode() IS bridged
-- (lua_creature_events.go:5014; the 2026-10-21 "no bridge" note is
-- stale) but this file has no UpdateAI tick surface to hang the
-- per-diff check on.
-- Cast-state: the boss's own UpdateAI pre-loop (:171) + in-loop
-- (:222) UNIT_STATE_CASTING gates are PORTED — every UpdateAI timer
-- arm (bloodthirst, conjure, vanish, startfeeding, donefeeding)
-- defers 500ms while casting, no repeat re-arm (mal_ganis idiom);
-- the pre-events.Update vanish-evade + _initialCheckTimer legs and
-- DamageTaken stay ungated per C++.
-- Unmodeled (documented, not wired):
-- Reset — CheckSpheres() evaluation PORTED on event 23 (shared helper
-- with the gossip path; true re-fires RemovePrison exactly as C++
-- does); the SummonCreatureGroup(CONTROLLERS) leg on false is PORTED
-- 2026-11-05 (bridge landed).
-- The ctor's SetDisableGravity(true) ports on ev5 (taldaramSpawn);
-- the RemovePrison SetDisableGravity(false) (:286) ports with
-- removePrison below; only MoveLand and the platform GO leg stay out.
-- npc_prince_taldaram_flame_sphere (30106 / 31686 / 31687 — entries
-- verifiable from the file enums, kalecgos pass) — PORTED via the boss
-- ev19 handler below (Reset casts + 3s move + 13s despawn, with the
-- SetGUID target stored per sphere); the whole functional AI ports —
-- SetGUID(_flameSphereTargetGUID) from the boss JustSummoned leg ports
-- via the ev19-stored target (flameSphereTarget, set at conjure-cast
-- time like C++); EVENT_START_MOVE's target lookup + GetAbsoluteAngle
-- MovePoint choreography ports via GetAngle + MoveTo (straight-line
-- spline, no PathGenerator — documented delta); EVENT_DESPAWN's
-- DespawnOrUnsummon leg is bridged
-- (creature:Despawn(), engine/world/lua_creature_events.go:236 — the old
-- "no despawn bridge (terestian precedent)" claim is stale).
-- go_prince_taldaram_sphere (193093 / 193094 Ancient Nerubian Device)
-- — PORTED (RegisterGameObjectGossipEvent, kirtonos-brazier
-- precedent): boss-alive gate via go:FindNearestCreature(29308, 100)
-- (same room); SetFlag NOT_SELECTABLE + SetGoState ACTIVE legs kept
-- verbatim (no-ops for static spawns — see keristrasza sphere note);
-- instance SetData(DATA_SPHERE_1/2, IN_PROGRESS) twin is boss
-- ScriptData via creature:SetData/GetData (per-creature store, read
-- by checkSpheres) PLUS the instance write below (data:SetData
-- feeds instance_ahnkahet.lua SpheresState save/load +
-- OnGameObjectCreate re-apply); boss
-- Talk(SAY_1 0) + CheckSpheres() -> RemovePrison() PORTED except
-- MoveLand (no bridge) and the platform HandleGameObject(true) leg
-- (static-GO state write, no-op) — both documented in removePrison.
-- spell_prince_taldaram_conjure_flame_sphere (55931: self-cast
-- 55895 + heroic 59511/59512) and
-- spell_prince_taldaram_flame_sphere_summon (55895/59511/59512:
-- destination Z offset) — no SpellScript binding bridge on the Lua
-- surface (razelikh precedent) — documented-only.
-- NOTE: the ICC Blood Prince Council's boss_prince_taldaram_icc is a
-- distinct registered script (boss_blood_prince_council.cpp, call 1349);
-- this lua covers only the Ahn'kahet boss_prince_taldaram registration.

local ENTRY_PRINCE_TALDARAM = 29308

local SAY_1 = 0 -- boss_prince_taldaram.cpp:67
local SAY_WARNING = 1 -- boss_prince_taldaram.cpp:68
local SAY_AGGRO = 2
local SAY_SLAY  = 3
local SAY_DEATH = 4
local SAY_FEED  = 5
local SAY_VANISH = 6

local SPELL_BLOODTHIRST          = 55968
local SPELL_CONJURE_FLAME_SPHERE = 55931
local SPELL_FLAME_SPHERE_SUMMON_1 = 55895
local SPELL_FLAME_SPHERE_SUMMON_2 = 59511
local SPELL_FLAME_SPHERE_SUMMON_3 = 59512
local SPELL_VANISH               = 55964
local SPELL_SHADOWSTEP           = 55966
local SPELL_EMBRACE_OF_THE_VAMPYR = 55959
local SPELL_BEAM_VISUAL = 60342 -- boss_prince_taldaram.cpp:50
local SPELL_FLAME_SPHERE_SPAWN_EFFECT = 55891 -- boss_prince_taldaram.cpp:38
local SPELL_FLAME_SPHERE_VISUAL = 55928 -- boss_prince_taldaram.cpp:39
local SPELL_FLAME_SPHERE_PERIODIC = 55926 -- boss_prince_taldaram.cpp:40
local SPELL_FLAME_SPHERE_DEATH_EFFECT = 55947 -- boss_prince_taldaram.cpp:41
local DATA_SPHERE_DISTANCE = 25.0 -- boss_prince_taldaram.cpp:61
local DATA_SPHERE_ANGLE_OFFSET = math.pi / 2 -- boss_prince_taldaram.cpp:62
local SPELL_HOVER_FALL = 60425 -- boss_prince_taldaram.cpp:51
local DATA_SPHERE_1 = 5 -- ahnkahet.h:38
local DATA_SPHERE_2 = 6 -- ahnkahet.h:39
local DATA_PRINCE_TALDARAM = 1 -- ahnkahet.h:32
local BOSS_STATE_DONE = 3 -- EncounterState DONE (InstanceScript.h:68-76)
local DATA_GROUND_POSITION_Z = 11.30809 -- boss_prince_taldaram.cpp:63
local GO_SPHERE_1 = 193093 -- ahnkahet.h:77
local GO_SPHERE_2 = 193094 -- ahnkahet.h:78
local GO_PRINCE_TALDARAM_PLATFORM = 193564 -- ahnkahet.h:76
local NPC_JEDOGA_CONTROLLER = 30181 -- ahnkahet.h:59
local NPC_FLAME_SPHERE_1 = 30106 -- boss_prince_taldaram.cpp:46
local NPC_FLAME_SPHERE_2 = 31686 -- boss_prince_taldaram.cpp:47
local NPC_FLAME_SPHERE_3 = 31687 -- boss_prince_taldaram.cpp:48
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000 -- UnitDefines.h:149
local GAMEOBJECT_FLAGS = 9 -- arlokk precedent
local GO_FLAG_NOT_SELECTABLE = 16 -- SharedDefines.h:1641
local GO_STATE_ACTIVE = 0

-- GUID-key rule: per-GUID tables are keyed on GetGUIDLow() (stable
-- number). GetGUID() returns a fresh UInt64 userdata per call and
-- Lua table keys compare userdata by identity, so a key stored
-- under one call never matches a lookup from another
-- (engine/scripting/object.go pushUInt64; instance_gundrak.lua
-- GUID-KEY FIX).
local timers = {}

-- C++ _embraceTargetGUID / _embraceTakenDamage: the target is stored as
-- the player object itself (the ObjectAccessor::GetUnit lookup has no
-- bridge, but the vanish pick already holds the object). Both clear on
-- engage/leave/died/reset (C++ Reset); DONE_FEEDING, the threshold trip
-- and KilledUnit clear the target ONLY — the counter persists across
-- feeds per C++ (it is never reset outside Reset).
local embraceTarget = {}
local embraceDamage = {}
local flameSphereTarget = {}

local function clearEmbrace(guid)
    embraceTarget[guid] = nil
    embraceDamage[guid] = nil
    -- C++ Reset clears _flameSphereTargetGUID too; leave/died
    -- clear it here as well (the next conjure re-latches it).
    flameSphereTarget[guid] = nil
end

-- C++ JustSummoned (:123-141) calls BossAI::JustSummoned, latching
-- every summon — in this model the conjure-cast flame spheres only
-- (controllers ride SummonCreatureGroup, PORTED 2026-11-05). Reset (_Reset)
-- and JustDied (_JustDied) both summons.DespawnAll().
local flameSpheres = {}

-- C++ JustSummoned SetGUID leg (:123-141): the sphere AI stores
-- _flameSphereTargetGUID for its 3s move; sphereTimers holds the
-- move/death events so died/reset/leave cancel them with the list.
local sphereTargets = {}
local sphereTimers = {}

local function cancelSphereTimers(sphereGuid)
    local ids = sphereTimers[sphereGuid]
    if ids then
        for _, id in pairs(ids) do
            RemoveEventById(id)
        end
        sphereTimers[sphereGuid] = nil
    end
end

local function despawnFlameSpheres(guid)
    local list = flameSpheres[guid]
    if list then
        for sphereGuid, summon in pairs(list) do
            cancelSphereTimers(sphereGuid)
            sphereTargets[sphereGuid] = nil
            summon:Despawn()
        end
        flameSpheres[guid] = nil
    end
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

-- C++ EVENT_BLOODTHIRST: DoCastSelf; 10s init, 10s repeat.
-- UpdateAI pre-loop (:171) + in-loop (:222) CASTING gates: while
-- casting the event stays queued — 500ms defer, no repeat re-arm
-- (mal_ganis idiom).
local function bloodthirstTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bloodthirst", 500, function()
            bloodthirstTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(creature, SPELL_BLOODTHIRST)
    schedule(guid, "bloodthirst", 10000, function()
        bloodthirstTick(creature, guid)
    end)
end

-- C++ EVENT_CONJURE_FLAME_SPHERES: DoCast(victim); 5s init, 15s repeat.
-- Same UpdateAI CASTING gates: defer precedes the victim pick and
-- the flame-sphere-target latch, so a deferred conjure latches
-- nothing until it actually fires.
local function conjureTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "conjure", 500, function()
            conjureTick(creature, guid)
        end)
        return
    end
    local victim = creature:GetVictim()
    if victim then
        -- C++ EVENT_CONJURE_FLAME_SPHERES (:182-189): the target
        -- GUID is latched BEFORE the cast, so JustSummoned can
        -- hand it to the spheres as they spawn.
        flameSphereTarget[guid] = victim
        creature:CastSpell(victim, SPELL_CONJURE_FLAME_SPHERE)
        -- spell_prince_taldaram_conjure_flame_sphere HandleScript: caster self-casts the summon(s).
        creature:CastSpell(creature, SPELL_FLAME_SPHERE_SUMMON_1, true)
        if creature:IsHeroic() then
            creature:CastSpell(creature, SPELL_FLAME_SPHERE_SUMMON_2, true)
            creature:CastSpell(creature, SPELL_FLAME_SPHERE_SUMMON_3, true)
        end
    end
    schedule(guid, "conjure", 15000, function()
        conjureTick(creature, guid)
    end)
end

-- C++ EVENT_DONE_FEEDING sits under the same UpdateAI pre-loop
-- CASTING gate: while casting the target clear waits, re-checked
-- every 500ms until it fires.
local function doneFeedingTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "donefeeding", 500, function()
            doneFeedingTick(creature, guid)
        end)
        return
    end
    embraceTarget[guid] = nil
end

-- C++ EVENT_START_FEEDING: RemoveAurasDueToSpell(VANISH) unconditionally,
-- then, if the embrace target still resolves, Shadowstep + Embrace casts
-- + Talk(SAY_FEED) + DONE_FEEDING in 20s. No alive check here (C++ has
-- none — GetEmbraceTarget returns the unit regardless of liveness).
local function startFeedingTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "startfeeding", 500, function()
            startFeedingTick(creature, guid)
        end)
        return
    end
    creature:RemoveAura(SPELL_VANISH)
    local target = embraceTarget[guid]
    if target then
        creature:CastSpell(target, SPELL_SHADOWSTEP)
        creature:CastSpell(target, SPELL_EMBRACE_OF_THE_VAMPYR)
        creature:Talk(SAY_FEED)
        schedule(guid, "donefeeding", 20000, function()
            doneFeedingTick(creature, guid)
        end)
    end
end

-- C++ EVENT_VANISH: gate GetThreatListSize() > 1 (GetAITargets twin);
-- pick SelectTarget(Random, 0, 100, true) = a random alive threat-list
-- player within 100yd. The pick is optional in C++ (the if only stores
-- the GUID): Talk + Vanish cast + START_FEEDING schedule fire even
-- when no target is picked. C++ DelayEvents(500ms) has no Lua twin.
local function vanishTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "vanish", 500, function()
            vanishTick(creature, guid)
        end)
        return
    end
    local targets = creature:GetAITargets()
    if #targets > 1 then
        local candidates = {}
        for _, t in ipairs(targets) do
            if t:GetObjectType() == "Player" and not t:IsDead() and creature:GetDistance(t) - creature:GetCombatReach() - 1.5 < 100 then
                candidates[#candidates + 1] = t
            end
        end
        if #candidates > 0 then
            embraceTarget[guid] = candidates[math.random(#candidates)]
        end
        creature:Talk(SAY_VANISH)
        creature:CastSpell(creature, SPELL_VANISH)
        schedule(guid, "startfeeding", 2000, function()
            startFeedingTick(creature, guid)
        end)
    end
    schedule(guid, "vanish", {25000, 35000}, function()
        vanishTick(creature, guid)
    end)
end

local function taldaramEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    clearEmbrace(guid)
    -- BossAI::JustEngagedWith base arms, base-first (C++ arm order);
    -- instance/setActive/ScheduleTasks legs out.
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    schedule(guid, "bloodthirst", 10000, function()
        bloodthirstTick(creature, guid)
    end)
    schedule(guid, "conjure", 5000, function()
        conjureTick(creature, guid)
    end)
    schedule(guid, "vanish", {25000, 35000}, function()
        vanishTick(creature, guid)
    end)
end

local function taldaramLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    despawnFlameSpheres(guid)
    cancelTimers(guid)
    clearEmbrace(guid)
end

local function taldaramTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        local guid = creature:GetGUIDLow()
        local target = embraceTarget[guid]
        if target ~= nil and victim:GetGUID() == target:GetGUID() then
            embraceTarget[guid] = nil
        end
        creature:Talk(SAY_SLAY)
    end
end

local function taldaramDied(event, creature, killer)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    clearEmbrace(guid)
    -- C++ BossAI::_JustDied (ScriptedCreature.cpp:513-520):
    -- instance->SetBossState(DATA_PRINCE_TALDARAM, DONE). The
    -- instance script's platform leg reads the flag below (no
    -- boss-state-change event exists — instance_ahnkahet.lua header).
    creature:SetBossState(DATA_PRINCE_TALDARAM, BOSS_STATE_DONE)
    local mapObj = creature:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        data.princeTaldaramDone = true
    end
    creature:Talk(SAY_DEATH)
    despawnFlameSpheres(guid)
end

local checkSpheres

local function taldaramReset(event, creature)
    local guid = creature:GetGUIDLow()
    despawnFlameSpheres(guid)
    cancelTimers(guid)
    clearEmbrace(guid)
    -- C++ Reset (:101-112): CheckSpheres() side effect (RemovePrison
    -- when both spheres set) ports via the shared helper below; the
    -- SummonCreatureGroup(CONTROLLERS) leg on false is PORTED 2026-11-05
    -- (bridge landed, SUMMON_GROUP_CONTROLLERS = 1, :58, :111).
    if not checkSpheres(creature) then
        creature:SummonCreatureGroup(1)
    end
end

-- C++ DamageTaken embrace latch: accumulate while the embrace target
-- resolves and is alive; past DATA_EMBRACE_DMG (20000 / heroic 40000)
-- clear the target + StopSpellCast (interrupts the live Embrace
-- 55959 channel — StopSpellCast bridged, lua_creature_events.go:1006).
-- Damage passes through unchanged (no return, garaxxas precedent).
local function taldaramDamageTaken(_, creature, _, damage)
    local guid = creature:GetGUIDLow()
    local target = embraceTarget[guid]
    if target and not target:IsDead() then
        local total = (embraceDamage[guid] or 0) + damage
        embraceDamage[guid] = total
        local threshold = 20000
        if creature:IsHeroic() then
            threshold = 40000
        end
        if total > threshold then
            embraceTarget[guid] = nil
            creature:StopSpellCast()
        end
    end
end

-- C++ boss_prince_taldaramAI ctor (boss_prince_taldaram.cpp:95-100):
-- me->SetDisableGravity(true) — the AI ctor runs once at AI creation;
-- ev5 (JustAppeared) is the Lua twin point (horseman/felmyst-trail
-- precedent). The _embraceTakenDamage/_initialCheckTimer inits are
-- internal (no UpdateAI vanish check is modeled — documented above).
local function taldaramSpawn(event, creature)
    creature:SetDisableGravity(true)
    -- C++ LoadObjectData (instance_ahnkahet.cpp:30-37): latch the
    -- boss GUID under DATA_PRINCE_TALDARAM (instance_guid_store.go;
    -- the instance-side OnCreatureCreate route is dead for static
    -- spawns — instance_ahnkahet.lua header).
    creature:SetGuidData(DATA_PRINCE_TALDARAM)
end

-- C++ boss_prince_taldaramAI::RemovePrison (:279-292) + CheckSpheres
-- (:261-269): both sphere data set -> free the boss. Leg-by-leg:
-- RemoveFlag NOT_SELECTABLE + RemoveAura(BEAM_VISUAL 60342) +
-- SetHomePosition(x, y, 11.30809, o) + DoCast(HOVER_FALL 60425) +
-- SetDisableGravity(false) + Talk(SAY_WARNING 1) all bridged;
-- summons.DespawnEntry(JEDOGA_CONTROLLER 30181) via
-- GetCreaturesInGrid + Despawn (anubarak mass-despawn precedent —
-- grid scope instead of summon-list scope; controllers exist only
-- as this boss's summons in the fight); MoveLand has no bridge
-- (the boss keeps its hover height — SetDisableGravity(false) is
-- still applied); platform HandleGameObject(true) = GO_STATE_ACTIVE
-- via FindNearestGameObject(193564, 100) + SetGoState (static-GO
-- state write, currently a no-op — kept verbatim for faithfulness).
local function removePrison(boss)
    boss:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
    for _, controller in ipairs(boss:GetCreaturesInGrid(NPC_JEDOGA_CONTROLLER, 100)) do
        controller:Despawn()
    end
    boss:RemoveAura(SPELL_BEAM_VISUAL)
    boss:SetHomePosition(boss:GetX(), boss:GetY(), DATA_GROUND_POSITION_Z, boss:GetO())
    boss:CastSpell(boss, SPELL_HOVER_FALL)
    boss:SetDisableGravity(false)
    boss:Talk(SAY_WARNING)
    local platform = boss:FindNearestGameObject(GO_PRINCE_TALDARAM_PLATFORM, 100)
    if platform then
        platform:SetGoState(GO_STATE_ACTIVE)
    end
end

checkSpheres = function(boss)
    if boss:GetData(DATA_SPHERE_1) == 0 or boss:GetData(DATA_SPHERE_2) == 0 then
        return false
    end
    removePrison(boss)
    return true
end

-- C++ go_prince_taldaram_sphere::OnGossipHello (:411-433): the whole
-- body sits inside the boss-alive gate (FindNearestCreature nil =
-- dead-or-absent, same gate as keristrasza); entry selects
-- DATA_SPHERE_1 (193093) vs DATA_SPHERE_2 (193094); Talk(SAY_1 0)
-- fires for both; C++ returns true (handled) == Lua true.
local function onTaldaramSphereGossip(_, _, go)
    local boss = go:FindNearestCreature(ENTRY_PRINCE_TALDARAM, 100)
    if not boss then
        return true
    end
    go:SetFlag(GAMEOBJECT_FLAGS, GO_FLAG_NOT_SELECTABLE)
    go:SetGoState(GO_STATE_ACTIVE)
    if go:GetEntry() == GO_SPHERE_1 then
        boss:SetData(DATA_SPHERE_1, 1)
    elseif go:GetEntry() == GO_SPHERE_2 then
        boss:SetData(DATA_SPHERE_2, 1)
    end
    -- C++ instance->SetData(DATA_SPHERE_1/2, IN_PROGRESS)
    -- (boss_prince_taldaram.cpp:422-427): feeds the instance
    -- script's SpheresState + GO re-apply + save
    -- (instance_ahnkahet.lua; instance_hooks.go SetData fires
    -- OnSetData event 8, the gundrak-altar precedent).
    local mapObj = go:GetMap()
    local data = mapObj and mapObj:GetInstanceData() or nil
    if data then
        if go:GetEntry() == GO_SPHERE_1 then
            data:SetData(DATA_SPHERE_1, 1)
        elseif go:GetEntry() == GO_SPHERE_2 then
            data:SetData(DATA_SPHERE_2, 1)
        end
    end
    boss:Talk(SAY_1)
    checkSpheres(boss)
    return true
end

RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 1, taldaramEnterCombat)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 19, function(_, creature, summoned)
    local entry = summoned:GetEntry()
    if entry ~= NPC_FLAME_SPHERE_1 and entry ~= NPC_FLAME_SPHERE_2
            and entry ~= NPC_FLAME_SPHERE_3 then
        return
    end
    local guid = creature:GetGUIDLow()
    local list = flameSpheres[guid]
    if not list then
        list = {}
        flameSpheres[guid] = list
    end
    list[summoned:GetGUIDLow()] = summoned
    -- C++ sphere Reset (:312-321): triggered spawn-effect +
    -- visual casts, then EVENT_START_MOVE (3s) + EVENT_DESPAWN
    -- (13s). The SetGUID leg lands here as the stored target.
    summoned:CastSpell(summoned, SPELL_FLAME_SPHERE_SPAWN_EFFECT, true)
    summoned:CastSpell(summoned, SPELL_FLAME_SPHERE_VISUAL, true)
    local sphereGuid = summoned:GetGUIDLow()
    sphereTargets[sphereGuid] = flameSphereTarget[guid]
    local offset = 0
    if entry == NPC_FLAME_SPHERE_2 then
        offset = DATA_SPHERE_ANGLE_OFFSET
    elseif entry == NPC_FLAME_SPHERE_3 then
        offset = -DATA_SPHERE_ANGLE_OFFSET
    end
    local ids = {}
    ids[1] = CreateLuaEvent(function()
        -- C++ EVENT_START_MOVE (:335-382): periodic cast first,
        -- then the move is target-gated (no target = no move).
        summoned:CastSpell(summoned, SPELL_FLAME_SPHERE_PERIODIC, true)
        local target = sphereTargets[sphereGuid]
        if target then
            local angle = summoned:GetAngle(target) + offset
            summoned:MoveTo(0,
                summoned:GetX() + DATA_SPHERE_DISTANCE * math.cos(angle),
                summoned:GetY() + DATA_SPHERE_DISTANCE * math.sin(angle),
                summoned:GetZ())
        end
    end, 3000)
    ids[2] = CreateLuaEvent(function()
        -- C++ EVENT_DESPAWN (:383-386): death-effect cast, then
        -- DespawnOrUnsummon(1s).
        summoned:CastSpell(summoned, SPELL_FLAME_SPHERE_DEATH_EFFECT, true)
        CreateLuaEvent(function()
            summoned:Despawn()
        end, 1000)
    end, 13000)
    sphereTimers[sphereGuid] = ids
end)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 2, taldaramLeaveCombat)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 3, taldaramTargetDied)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 4, taldaramDied)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 9, taldaramDamageTaken)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 5, taldaramSpawn)
RegisterCreatureEvent(ENTRY_PRINCE_TALDARAM, 23, taldaramReset)

RegisterGameObjectGossipEvent(GO_SPHERE_1, 1, onTaldaramSphereGossip)
RegisterGameObjectGossipEvent(GO_SPHERE_2, 1, onTaldaramSphereGossip)
