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
--   gossip flag on arrival) is NOT duplicated here — it ports
--   in instance_oculus.lua (FreeDragons arrival poll).
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
--   leg rides a bounded 1s arrival poll (no ev28 in Go).
-- - spell_oculus_stop_time (49838, AuraScript AfterEffectApply,
--   oculus.cpp:560-583): the stun apply casts
--   SPELL_AMBER_SHOCK_CHARGE (49836) 5x on the target — ports
--   on AuraEventOnApply (2), the apply twin.
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
--   +2s despawn + takeoff spline): no PassengerBoarded bridge,
--   so the board trigger — and everything gated on it — stays
--   out.
-- - The remaining SpellScripts/AuraScripts: call-drake dest
--   select (49345/49461/49462, SpellScript), shock lance
--   damage calc (49840, SpellScript), touch the nightmare
--   damage calc (50341, SpellScript) — no SpellScript bridge;
--   evasive maneuvers proc (50240) + temporal rift proc
--   (49592) — no AuraEvent proc twin (events are 1 periodic /
--   2 apply / 3 remove / 4 stack-change only); dream funnel
--   calc-amount (50344) — event 4 is stack-count change, not
--   DoEffectCalcAmount.
-- - Drake MovementInform POINT_LAND in its C++ form (no ev28);
--   the arrival-poll twin ports above.

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

local POINT_LAND = 2 -- oculus.cpp:121

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

-- C++ SpellHit (oculus.cpp:289-310): creator must exist, spell
-- must be GPS; the whisper chain reads boss states first, then
-- the construct kill-state (GetData(DATA_CONSTRUCTS) — 0 while
-- the count is 0, 1 at one construct, 2 above; the C++ default
-- for an unlatched counter is KILL_NO_CONSTRUCT 0).
local function drakeSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_GPS then
        return
    end
    local creator = drakeCreators[creature:GetGUID()]
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

-- C++ MovementInform POINT_LAND twin (oculus.cpp:345-349):
-- SetDisableGravity(false) on arrival at the land position.
-- Bounded 30s poll (freeDragons idiom, instance_oculus.lua).
local function drakeLandPoll(creature, guid, x, y, z, tries)
    local dx = creature:GetX() - x
    local dy = creature:GetY() - y
    local dz = creature:GetZ() - z
    if dx * dx + dy * dy + dz * dz < 4.0 then
        creature:SetDisableGravity(false)
        return
    end
    if tries <= 0 then
        return
    end
    CreateLuaEvent(function()
        drakeLandPoll(creature, guid, x, y, z, tries - 1)
    end, 1000, 1)
end

-- C++ IsSummonedBy (oculus.cpp:312-343).
local function drakeSummoned(event, creature, summoner)
    if not summoner then
        return
    end
    drakeCreators[creature:GetGUID()] = summoner
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
    local x = summoner:GetX()
    local y = summoner:GetY()
    local z = summoner:GetZ()
    creature:MoveTo(POINT_LAND, x, y, z)
    drakeLandPoll(creature, creature:GetGUID(), x, y, z, 30)
end

local DRAKE_ENTRIES = { NPC_RUBY_DRAKE_VEHICLE, NPC_EMERALD_DRAKE_VEHICLE, NPC_AMBER_DRAKE_VEHICLE }
for _, entry in ipairs(DRAKE_ENTRIES) do
    RegisterCreatureEvent(entry, 14, drakeSpellHit)
    RegisterCreatureEvent(entry, 22, drakeSummoned)
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

RegisterAuraEvent(SPELL_STOP_TIME, 2, stopTimeApply)
