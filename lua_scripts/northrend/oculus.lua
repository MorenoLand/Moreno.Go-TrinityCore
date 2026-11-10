-- The Oculus zone script — Lua port of
-- src/server/scripts/Northrend/Nexus/Oculus/oculus.cpp
-- (AddSC_oculus, the non-instance Oculus group).
-- Ported arms (C++-exact unless noted):
-- - spell_oculus_ride_ruby_emerald_amber_drake_que (AuraScript
--   periodic, oculus.cpp:449-466): the que-aura target casts the
--   DBC trigger of EFFECT_0 triggered on the aura's caster.
-- - npc_verdisa_beglaristrasz_eternos (27657/27658/27659):
--   OnGossipSelect essence swap (oculus.cpp:150-191). No hello
--   hook is registered on purpose: the DB menus (9573/9574/9575)
--   load by default, and registering a hello hook would veto
--   them (gossip.go hello-veto arm); only the select hook
--   (event 2) is bound, and listID is the C++ gossipListId
--   (gossip.go select args). The swap closes the gossip
--   (player:GossipComplete, the C++ SendCloseGossip twin).
--   MovementInform POINT_MOVE_OUT (Belgaristrasz Talk 0 +
--   gossip flag on arrival) ports in instance_oculus.lua
--   (ev28 dragonMoveOutInform).
-- - npc_ruby_emerald_amber_drake (27756/27692/27755):
--   SpellHit GPS (oculus.cpp:289-310, ev14 — the fire site is
--   live, boss_ai.go:3813): the whisper target is the drake's
--   creator, latched at summon time below (no GUID->creature
--   lookup bridge on the Lua surface — midnight precedent);
--   boss-state chain + DATA_CONSTRUCTS read from the shared
--   instanceData table (instance_oculus.lua).
--   IsSummonedBy (oculus.cpp:312-343, ev22): Eregos
--   IN_PROGRESS -> despawn nearest Eregos in 450yd,
--   SetFacingToObject(summoner), per-entry ride-que cast on
--   the summoner, MoveTo(POINT_LAND) at the summoner position;
--   the MovementInform POINT_LAND -> SetDisableGravity(false)
--   leg ports on ev28 (drakeLandInform below).
-- - spell_oculus_stop_time (49838, AuraScript AfterEffectApply,
--   oculus.cpp:560-583): the stun apply casts
--   SPELL_AMBER_SHOCK_CHARGE (49836) 5x on the target — ports
--   on AuraEventOnApply (2), the apply twin.
-- - spell_oculus_evasive_maneuvers (50240, AuraScript OnEffectProc,
--   oculus.cpp:484-493) + spell_oculus_temporal_rift (49592,
--   AuraScript OnEffectProc, oculus.cpp:599-621): both port on
--   AuraEventOnProc (6), the woe-strike/koralon precedent —
--   return true is PreventDefaultAction. The header's old "no
--   AuraEvent proc twin" claim is stale: fireAuraProc
--   (lua_aura.go) fires from the creature proc loop
--   (boss_ai.go) after the DBC proc-entry gates, like the
--   session loop. DORMANT until Spell.dbc ships the aura +
--   proc records (koralon precedent): the handler fires only
--   when the generated proc entry matches the event (dodge/
--   parry taken for 50240, damage taken for 49592).
-- Unmodeled (documented-only, no bridges):
-- - npc_image_belgaristrasz: ENTRY-BLOCKED — no summon anywhere
--   in the C++ tree, no DB row in-tree (world.db is an empty
--   stub), two web searches (Wowhead/Wowpedia) returned no entry
--   id (frogger precedent). Spec-ready: ev22 IsSummonedBy —
--   summoner entry Varos (27447) -> Talk(SAY_VAROS 0),
--   summoner entry Urom (27655) -> Talk(SAY_UROM 1), then
--   DespawnOrUnsummon(60s) (oculus.cpp:226-237).
-- - Drake PassengerBoarded + UpdateAI whisper machine
--   (oculus.cpp:345-423: WELCOME 10s / ABILITIES +5s /
--   SPECIAL_ATTACK 10s / LOW_HEALTH 40% + 25s reset / TAKE_OFF
--   +2s despawn + takeoff spline): PORTED 2026-11-09 (tick 512)
--   via RegisterVehicleEvent 5 (add) / 6 (remove), kologarn
--   precedent; the UpdateAI health check rides a 1s CreateLuaEvent
--   pump (no UpdateAI tick surface — check granularity is the one
--   deviation). SetSpeedRate(MOVE_FLIGHT, 1.0) stays out (the
--   bridge rejects non-RUN types; 1.0 is the default flight rate,
--   so the leg is a no-op); SetOrientation(2.5) rides
--   SetFacingTo(2.5), the absolute-facing twin.
-- - The remaining SpellScripts/AuraScripts: shock lance damage
--   calc (49840) ports Go-side
--   (spell_onhit.go shockLanceDetonate: base + 49836 eff0 amount
--   from this caster, charge aura consumed; creature + player
--   victims — player leg reads the victim session's activeAuras);
--   touch the nightmare damage calc (50341) ports Go-side
--   (spell_onhit.go onHitDamageRewrite: base + 30% caster max HP).
--   dream funnel calc-amount (50344) — event 4 is stack-count
--   change, not DoEffectCalcAmount.
-- - Call-drake dest select (49345/49461/49462, SpellScript
--   OnDestinationTargetSelect, oculus.cpp:424-437: +12z summon
--   offset) — no bridge: DestinationTargetSelect handlers are the
--   documented no-bridge leg (target_dest_targets.go:470, no
--   ScriptMgr model); the summon itself rides the Go spell system
--   at the unadjusted caster-front dest.
-- - Drake MovementInform POINT_LAND now ports on ev28
--   (drakeLandInform below).

-- C++ spell_oculus_ride_ruby_emerald_amber_drake_que::HandlePeriodic
-- (oculus.cpp:449-466): PreventDefaultAction; the aura target casts
-- the DBC trigger of EFFECT_0 (aura:GetTriggerSpell(0)) triggered on
-- the aura's caster — the C++ comment notes the default trigger's
-- caster is wrong, so the handler re-casts it explicitly. One
-- handler for all three drake-que auras (49427 emerald, 49459 amber,
-- 49463 ruby). Returns true, the PreventDefaultAction twin, so the
-- Go default trigger cast does not also fire.
local SPELL_RIDE_EMERALD_DRAKE_QUE = 49427
local SPELL_RIDE_AMBER_DRAKE_QUE = 49459
local SPELL_RIDE_RUBY_DRAKE_QUE = 49463

local function drakeQueTick(event, aura, target)
    local trigger = aura:GetTriggerSpell(0)
    if trigger ~= 0 then
        local caster = aura:GetCaster()
        if caster then
            target:CastSpell(caster, trigger, true)
        end
    end
    return true
end

RegisterAuraEvent(SPELL_RIDE_EMERALD_DRAKE_QUE, 1, drakeQueTick)
RegisterAuraEvent(SPELL_RIDE_AMBER_DRAKE_QUE, 1, drakeQueTick)
RegisterAuraEvent(SPELL_RIDE_RUBY_DRAKE_QUE, 1, drakeQueTick)

local NPC_VERDISA = 27657 -- oculus.h:45
local NPC_BELGARISTRASZ = 27658 -- oculus.h:46
local NPC_ETERNOS = 27659 -- oculus.h:47

local NPC_RUBY_DRAKE_VEHICLE = 27756 -- oculus.h:44
local NPC_EMERALD_DRAKE_VEHICLE = 27692 -- oculus.h:45
local NPC_AMBER_DRAKE_VEHICLE = 27755 -- oculus.h:46
local NPC_EREGOS = 27656 -- oculus.h:40

local ITEM_EMERALD_ESSENCE = 37815 -- oculus.cpp:41
local ITEM_AMBER_ESSENCE = 37859 -- oculus.cpp:42
local ITEM_RUBY_ESSENCE = 37860 -- oculus.cpp:43

local SPELL_GPS = 53389 -- oculus.cpp:119
local SPELL_AMBER_SHOCK_CHARGE = 49836 -- oculus.cpp:75
local SPELL_STOP_TIME = 49838 -- oculus.cpp:555

local DATA_VAROS = 1 -- oculus.h:38
local DATA_UROM = 2 -- oculus.h:39
local DATA_EREGOS = 3 -- oculus.h:40

local BOSS_STATE_IN_PROGRESS = 1 -- EncounterState (InstanceScript.h:68-76)
local BOSS_STATE_DONE = 3 -- EncounterState (InstanceScript.h:68-76)

local WHISPER_GPS_10_CONSTRUCTS = 5 -- oculus.cpp:129
local WHISPER_GPS_1_CONSTRUCT = 6 -- oculus.cpp:130
local WHISPER_GPS_VAROS = 7 -- oculus.cpp:131
local WHISPER_GPS_UROM = 8 -- oculus.cpp:132
local WHISPER_GPS_EREGOS = 9 -- oculus.cpp:133
local WHISPER_GPS_END = 10 -- oculus.cpp:134

local SAY_DRAKES_TAKEOFF = 0 -- oculus.cpp:109
local WHISPER_DRAKES_WELCOME = 1 -- oculus.cpp:110
local WHISPER_DRAKES_ABILITIES = 2 -- oculus.cpp:111
local WHISPER_DRAKES_SPECIAL = 3 -- oculus.cpp:112
local WHISPER_DRAKES_LOWHEALTH = 4 -- oculus.cpp:113

local POINT_LAND = 2 -- oculus.cpp:121
local POINT_TAKE_OFF = 3 -- oculus.cpp:121

-- C++ OnGossipSelect (oculus.cpp:150-191). StoreEssence is
-- player:AddItem (the CanStoreNewItem gate reads nil on failure,
-- same observable); RemoveEssence is player:DestroyItemCount
-- (itemId, 1, true, false — the Go twin takes the first three).
local function dragonGossipSelect(event, player, creature, sender, action, code, listId)
    local entry = creature:GetEntry()
    if entry == NPC_VERDISA then
        if listId < 1 or listId > 3 then
            return false
        end
        if listId == 2 then
            player:DestroyItemCount(ITEM_AMBER_ESSENCE, 1, true)
        elseif listId == 3 then
            player:DestroyItemCount(ITEM_RUBY_ESSENCE, 1, true)
        end
        player:AddItem(ITEM_EMERALD_ESSENCE, 1)
    elseif entry == NPC_ETERNOS then
        if listId < 1 or listId > 3 then
            return false
        end
        if listId == 2 then
            player:DestroyItemCount(ITEM_EMERALD_ESSENCE, 1, true)
        elseif listId == 3 then
            player:DestroyItemCount(ITEM_RUBY_ESSENCE, 1, true)
        end
        player:AddItem(ITEM_AMBER_ESSENCE, 1)
    elseif entry == NPC_BELGARISTRASZ then
        if listId > 2 then
            return false
        end
        if listId == 1 then
            player:DestroyItemCount(ITEM_AMBER_ESSENCE, 1, true)
        elseif listId == 2 then
            player:DestroyItemCount(ITEM_EMERALD_ESSENCE, 1, true)
        end
        player:AddItem(ITEM_RUBY_ESSENCE, 1)
    else
        return false
    end
    player:GossipComplete()
end

RegisterCreatureGossipEvent(NPC_VERDISA, 2, dragonGossipSelect)
RegisterCreatureGossipEvent(NPC_ETERNOS, 2, dragonGossipSelect)
RegisterCreatureGossipEvent(NPC_BELGARISTRASZ, 2, dragonGossipSelect)

-- Drake creator latch: ev22 stores the summoner (the C++ creator)
-- per drake GUID; ev14 reads it back for the GPS whisper target.
local drakeCreators = {}

-- Drake event timers (PassengerBoarded / UpdateAI machine,
-- oculus.cpp:345-423), keyed on GetGUIDLow() (stable uint32 — the
-- GUID-key rule: GetGUID() userdata never matches across handlers).
local drakeTimers = {}
local drakeHealthWarning = {}
local drakeHealthPump = {}

local function drakeCancelTimers(guid)
    local per = drakeTimers[guid]
    if per then
        for _, id in pairs(per) do
            RemoveEventById(id)
        end
        drakeTimers[guid] = nil
    end
end

local function drakeSchedule(guid, key, delay, fn)
    local per = drakeTimers[guid]
    if not per then
        per = {}
        drakeTimers[guid] = per
    end
    if per[key] then
        RemoveEventById(per[key])
    end
    per[key] = CreateLuaEvent(fn, delay)
end

local function drakeCancelHealthPump(guid)
    local id = drakeHealthPump[guid]
    if id then
        RemoveEventById(id)
        drakeHealthPump[guid] = nil
    end
end

-- C++ UpdateAI health leg (oculus.cpp:352-356): _healthWarning
-- gates a <= 40% check every AI tick. No UpdateAI tick surface
-- exists in Lua, so a 1s CreateLuaEvent pump is the twin (check
-- granularity is the one deviation); the pump is NOT part of the
-- event map, so PassengerBoarded-remove does not cancel it — only
-- leave/died/reset do.
local function drakeHealthCheckTick(creature, guid)
    if drakeHealthWarning[guid] and creature:GetHealthPct() <= 40 then
        local creator = drakeCreators[guid]
        if creator then
            creature:Talk(WHISPER_DRAKES_LOWHEALTH, creator)
        end
        drakeHealthWarning[guid] = false
        drakeSchedule(guid, "resetLowHealth", 25000, function()
            drakeHealthWarning[guid] = true
        end)
    end
    drakeHealthPump[guid] = CreateLuaEvent(function()
        drakeHealthCheckTick(creature, guid)
    end, 1000)
end

local function drakeSeedHealthPump(creature, guid)
    drakeCancelHealthPump(guid)
    drakeHealthWarning[guid] = true
    drakeHealthPump[guid] = CreateLuaEvent(function()
        drakeHealthCheckTick(creature, guid)
    end, 1000)
end

-- C++ SpellHit (oculus.cpp:289-310): creator must exist, spell
-- must be GPS; the whisper chain reads boss states first, then
-- the construct kill-state (GetData(DATA_CONSTRUCTS) — 0 while
-- the count is 0, 1 at one construct, 2 above; the C++ default
-- for an unlatched counter is KILL_NO_CONSTRUCT 0).
local function drakeSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_GPS then
        return
    end
    local creator = drakeCreators[creature:GetGUIDLow()]
    if not creator then
        return
    end
    if creature:GetBossState(DATA_EREGOS) == BOSS_STATE_DONE then
        creature:Talk(WHISPER_GPS_END, creator)
    elseif creature:GetBossState(DATA_UROM) == BOSS_STATE_DONE then
        creature:Talk(WHISPER_GPS_EREGOS, creator)
    elseif creature:GetBossState(DATA_VAROS) == BOSS_STATE_DONE then
        creature:Talk(WHISPER_GPS_UROM, creator)
    else
        local count = 0
        local mapObj = creature:GetMap()
        if mapObj then
            local data = mapObj:GetInstanceData()
            if data then
                count = data.constructCount or 0
            end
        end
        if count == 0 then
            creature:Talk(WHISPER_GPS_VAROS, creator)
        elseif count == 1 then
            creature:Talk(WHISPER_GPS_1_CONSTRUCT, creator)
        else
            creature:Talk(WHISPER_GPS_10_CONSTRUCTS, creator)
        end
    end
end

-- C++ MovementInform POINT_LAND (oculus.cpp:316-320, event 28):
-- SetDisableGravity(false) on arrival at the land position.
local POINT_MOTION_TYPE = 8

local function drakeLandInform(_, creature, moveType, id)
    if moveType == POINT_MOTION_TYPE and id == POINT_LAND then
        creature:SetDisableGravity(false)
    end
end

-- C++ npc_ruby_emerald_amber_drakeAI::PassengerBoarded +
-- UpdateAI (oculus.cpp:322-423) via RegisterVehicleEvent 5 (add) /
-- 6 (remove), kologarn precedent. The handler shape is
-- (event, vehicle, passenger, seatId); apply rides the event id.
-- The C++ passenger gate is TYPEID_PLAYER; the Lua twin is
-- GetObjectType() == "Player".
local function drakeWelcomeTick(vehicle, guid)
    local creator = drakeCreators[guid]
    if creator then
        vehicle:Talk(WHISPER_DRAKES_WELCOME, creator)
    end
    drakeSchedule(guid, "abilities", 5000, function()
        local creator2 = drakeCreators[guid]
        if creator2 then
            vehicle:Talk(WHISPER_DRAKES_ABILITIES, creator2)
        end
    end)
end

local function drakeSpecialTick(vehicle, guid)
    local creator = drakeCreators[guid]
    if creator then
        vehicle:Talk(WHISPER_DRAKES_SPECIAL, creator)
    end
end

local function drakeTakeOffTick(vehicle, guid)
    -- C++ EVENT_TAKE_OFF (oculus.cpp:397-410) in C++ order:
    -- DespawnOrUnsummon(2050ms), SetOrientation(2.5) via the
    -- absolute-facing twin SetFacingTo, Talk, position + offset
    -- (10, 10, 12), SetDisableGravity(true), MovePoint.
    -- SetSpeedRate(MOVE_FLIGHT, 1.0) stays out: the bridge rejects
    -- non-RUN types, and 1.0 is the default flight rate (no-op).
    vehicle:DespawnOrUnsummon(2050)
    vehicle:SetFacingTo(2.5)
    vehicle:Talk(SAY_DRAKES_TAKEOFF)
    vehicle:SetDisableGravity(true)
    vehicle:MoveTo(POINT_TAKE_OFF, vehicle:GetX() + 10, vehicle:GetY() + 10, vehicle:GetZ() + 12)
end

local function drakePassengerBoarded(event, vehicle, passenger, seatId)
    local entry = vehicle:GetEntry()
    if entry ~= NPC_RUBY_DRAKE_VEHICLE and entry ~= NPC_EMERALD_DRAKE_VEHICLE and entry ~= NPC_AMBER_DRAKE_VEHICLE then
        return
    end
    if passenger == nil or passenger:GetObjectType() ~= "Player" then
        return
    end
    local guid = vehicle:GetGUIDLow()
    local apply = event == 5
    if apply then
        if vehicle:GetBossState(DATA_VAROS) ~= BOSS_STATE_DONE then
            drakeSchedule(guid, "welcome", 10000, function()
                drakeWelcomeTick(vehicle, guid)
            end)
        elseif vehicle:GetBossState(DATA_UROM) == BOSS_STATE_DONE then
            drakeSchedule(guid, "special", 10000, function()
                drakeSpecialTick(vehicle, guid)
            end)
        end
    else
        -- C++ PassengerBoarded apply=false: _events.Reset() +
        -- EVENT_TAKE_OFF in 2s. The health pump is not part of the
        -- event map and keeps running until leave/died/reset.
        drakeCancelTimers(guid)
        drakeSchedule(guid, "takeoff", 2000, function()
            drakeTakeOffTick(vehicle, guid)
        end)
    end
end

RegisterVehicleEvent(5, drakePassengerBoarded)
RegisterVehicleEvent(6, drakePassengerBoarded)

-- C++ Reset (oculus.cpp:263-267): _events.Reset() + Initialize()
-- (_healthWarning = true). ev23 pre-fires on death, so the pump
-- re-seed is alive-guarded (prowlerReset convention).
local function drakeReset(event, creature)
    local guid = creature:GetGUIDLow()
    drakeCancelTimers(guid)
    drakeCancelHealthPump(guid)
    if creature:IsAlive() then
        drakeSeedHealthPump(creature, guid)
    else
        drakeHealthWarning[guid] = nil
    end
end

local function drakeLeaveOrDied(event, creature)
    local guid = creature:GetGUIDLow()
    drakeCancelTimers(guid)
    drakeCancelHealthPump(guid)
    drakeHealthWarning[guid] = nil
end

-- C++ IsSummonedBy (oculus.cpp:312-343).
local function drakeSummoned(event, creature, summoner)
    if not summoner then
        return
    end
    drakeCreators[creature:GetGUIDLow()] = summoner
    -- C++ ctor Initialize() (oculus.cpp:256-260): _healthWarning
    -- starts true; the UpdateAI health leg rides the 1s pump.
    drakeSeedHealthPump(creature, creature:GetGUIDLow())
    if creature:GetBossState(DATA_EREGOS) == BOSS_STATE_IN_PROGRESS then
        local eregos = creature:FindNearestCreature(NPC_EREGOS, 450)
        if eregos then
            eregos:DespawnOrUnsummon()
        end
    end
    creature:SetFacingToObject(summoner)
    local entry = creature:GetEntry()
    if entry == NPC_RUBY_DRAKE_VEHICLE then
        creature:CastSpell(summoner, SPELL_RIDE_RUBY_DRAKE_QUE, true)
    elseif entry == NPC_EMERALD_DRAKE_VEHICLE then
        creature:CastSpell(summoner, SPELL_RIDE_EMERALD_DRAKE_QUE, true)
    elseif entry == NPC_AMBER_DRAKE_VEHICLE then
        creature:CastSpell(summoner, SPELL_RIDE_AMBER_DRAKE_QUE, true)
    else
        return
    end
    creature:MoveTo(POINT_LAND, summoner:GetX(), summoner:GetY(), summoner:GetZ())
end

local DRAKE_ENTRIES = { NPC_RUBY_DRAKE_VEHICLE, NPC_EMERALD_DRAKE_VEHICLE, NPC_AMBER_DRAKE_VEHICLE }
for _, entry in ipairs(DRAKE_ENTRIES) do
    RegisterCreatureEvent(entry, 14, drakeSpellHit)
    RegisterCreatureEvent(entry, 22, drakeSummoned)
    RegisterCreatureEvent(entry, 28, drakeLandInform)
    RegisterCreatureEvent(entry, 23, drakeReset)
    RegisterCreatureEvent(entry, 2, drakeLeaveOrDied)
    RegisterCreatureEvent(entry, 4, drakeLeaveOrDied)
end

-- C++ spell_oculus_stop_time::Apply (oculus.cpp:560-583):
-- AfterEffectApply on the stun — the caster casts
-- SPELL_AMBER_SHOCK_CHARGE 5x on the target.
local function stopTimeApply(event, aura, target)
    local caster = aura:GetCaster()
    if not caster then
        return
    end
    for _ = 1, 5 do
        caster:CastSpell(target, SPELL_AMBER_SHOCK_CHARGE, true)
    end
end

local SPELL_EVASIVE_MANEUVERS = 50240 -- oculus.cpp:57
local SPELL_EVASIVE_CHARGES = 50241 -- oculus.cpp:56
local SPELL_TEMPORAL_RIFT = 49592 -- oculus.cpp:71

-- C++ spell_oculus_evasive_maneuvers::HandleProc
-- (oculus.cpp:484-493): OnEffectProc, EFFECT_2,
-- SPELL_AURA_PROC_TRIGGER_SPELL. PreventDefaultAction; the
-- holder loses one Evasive Charge stack
-- (RemoveAuraFromStack: ModStackAmount(-1), aura removed at 0);
-- when no charges aura remains, the maneuvers aura removes
-- itself (Remove()). The charges aura is read through the live
-- wire count (aura:GetStackAmount); SetStackAmount(0) does not
-- remove in Go, so the last stack goes through RemoveAura.
local function evasiveManeuversProc(event, aura, target, actor, procTarget, damage)
    if target ~= nil then
        local charges = target:GetAura(SPELL_EVASIVE_CHARGES)
        if charges ~= nil then
            local stacks = charges:GetStackAmount()
            if stacks <= 1 then
                target:RemoveAura(SPELL_EVASIVE_CHARGES)
            else
                charges:SetStackAmount(stacks - 1)
            end
        end
        if not target:HasAura(SPELL_EVASIVE_CHARGES) then
            aura:Remove()
        end
    end
    return true
end

RegisterAuraEvent(SPELL_EVASIVE_MANEUVERS, 6, evasiveManeuversProc)

-- C++ spell_oculus_temporal_rift::HandleProc
-- (oculus.cpp:599-621): OnEffectProc, EFFECT_2, SPELL_AURA_DUMMY.
-- PreventDefaultAction; a zero-damage event does nothing; else the
-- damage accumulates into the effect-2 amount, and each 15000
-- threshold makes the aura caster cast SPELL_AMBER_SHOCK_CHARGE
-- (49836) triggered on the holder, carrying the remainder forward
-- (one threshold per proc, the C++ single-subtract arm).
local function temporalRiftProc(event, aura, target, actor, procTarget, damage)
    if damage == nil or damage == 0 then
        return true
    end
    local amount = aura:GetAmount(2) + damage
    if amount >= 15000 then
        local caster = aura:GetCaster()
        if caster ~= nil and target ~= nil then
            caster:CastSpell(target, SPELL_AMBER_SHOCK_CHARGE, true)
        end
        amount = amount - 15000
    end
    aura:SetAmount(amount, 2)
    return true
end

RegisterAuraEvent(SPELL_TEMPORAL_RIFT, 6, temporalRiftProc)
