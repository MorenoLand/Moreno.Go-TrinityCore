-- Azuremyst Isle: Engineer Spark Overgrind — Lua port of
-- src/server/scripts/Kalimdor/zone_azuremyst_isle.cpp
-- (npc_engineer_spark_overgrindAI : public ScriptedAI; registered in
-- AddSC_azuremyst_isle(), kalimdor loader decl :108).
-- Entry: 17243 (NPC_SPARK, in-enum in the Geezle enum block).
-- Ported arms:
-- - JustEngagedWith -> Talk(ATTACK_YELL = 2) on the engager.
-- - Dynamite_Timer: DoCastVictim(SPELL_DYNAMITE = 7978), init 8000
--   -> 8000, gated on creature:GetVictim() (C++ UpdateVictim gate);
--   timers via CreateLuaEvent, scheduled on enter combat, cancelled
--   on leave combat / reset (boss_ouro precedent).
-- - Emote_Timer (out-of-combat only): Talk(SAY_TEXT = 0) +
--   Talk(SAY_EMOTE = 1), init urand(120000,150000) -> same range;
--   armed at spawn (event 23 fires on spawn), cancelled on enter
--   combat, re-armed on leave combat. C++ resumes a partially
--   elapsed timer after combat; the Lua port re-arms full delay.
-- - DoMeleeAttackIfReady — engine-driven in Go (ouro precedent).
-- - IsTreeEvent latch (GetAreaId() == AREA_COVE 3579 / AREA_ISLE
--   3639, zone_azuremyst_isle.cpp:213-216, latched at construct and
--   Reset): creature:GetAreaId() (engine/world/lua_creature_events.go,
--   Map::GetZoneAndAreaId twin). The dynamite seed (enter combat) and
--   the emote re-arms (leave combat / reset) skip when the latch is
--   set — C++ UpdateAI returns before both pumps when IsTreeEvent
--   (:251-252), so the tree-event Spark never casts or emotes;
--   JustEngagedWith's Talk stays unconditional (:227-230). Known
--   engine-driven deviation: C++ also skips DoMeleeAttackIfReady on
--   the early return, but melee is engine-driven in Go (ouro
--   precedent) and not Lua-gatable — a deliberately attacked
--   tree-event Spark fights back in Go where C++ stands still.
-- - Reset's UNIT_NPC_FLAGS restore (zone_azuremyst_isle.cpp:224)
--   + SetFaction(NormFaction) restore (:223): constructor-captured
--   values cached per GUID, restored via creature:SetNPCFlags()
--   (engine/world/lua_objects.go:3274) and creature:SetFaction()
--   (lua_creature_events.go:6712) — C++ arm order after
--   Initialize().
-- - OnGossipSelect (zone_azuremyst_isle.cpp:232-239): PORTED below
--   — player:GossipComplete() (CloseGossipMenuFor twin, rizzle
--   precedent) + SetFaction(FACTION_MONSTER = 14,
--   SharedDefines.h:226) + creature:Attack(player, true)
--   (lua_creature_events.go:1945, arlokk precedent); returns
--   false, C++-exact.
-- Documented-only (no bridges):
-- - The rest of zone_azuremyst_isle.cpp, documented-only:
--   npc_draenei_survivor — Reset ported in
--   npc_draenei_survivor.lua (entry 16483, Wowhead-verified); the
--   MoveInLineOfSight help-call, SpellHit heal leg and timed
--   thanks/run-away machine stay out (no SpellInfo / MovePoint /
--   ev27-engage-only bridges).
--   npc_injured_draenei — Reset ported in npc_injured_draenei.lua
--   (entry 16971, Wowhead-verified); that Reset is the whole script.
--   npc_magwin — EscortAI (OnQuestAccept / WaypointReached /
--   events): escort IS bridged now (StartEscort / MoveWaypoint /
--   GetEscortPlayer / StopEscort, ev6 + ev31 — ruul port), so the
--   old "no escort bridge" claim is stale; magwin needs its own
--   audit.
--   npc_geezle — creature:SummonCreature IS bridged
--   (lua_creature_events.go:4161) and used in-tree (ruul wp13/wp19
--   waves), so the old "zero Lua usage" claim is stale; the
--   MotionMaster / quest-status / gameobject-respawn legs still
--   need their own audit.
--   spell_inoculate_nestlewood — AuraScript/PeriodicTick: the
--   bridge exists (RegisterAuraEvent ev1); its arms are unassessed
--   here, so it stays out pending its own audit (the ashenvale
--   precedent is stale — row 320 stays out on its unidentifiable
--   spell ID, not on the bridge).
local ENTRY = 17243

local SAY_TEXT = 0
local SAY_EMOTE = 1
local ATTACK_YELL = 2

local SPELL_DYNAMITE = 7978

local FACTION_MONSTER = 14 -- SharedDefines.h:226

local DYNAMITE_DELAY_MS = 8000
local EMOTE_MIN_MS = 120000
local EMOTE_MAX_MS = 150000

local timers = {}

-- guid -> AI-constructor-captured UNIT_NPC_FLAGS
-- (zone_azuremyst_isle.cpp:205: NpcFlags = creature->GetUInt32Value(UNIT_NPC_FLAGS))
local npcFlags = {}

-- guid -> AI-constructor-captured faction
-- (zone_azuremyst_isle.cpp:204: NormFaction = creature->GetFaction())
local normFaction = {}

-- guid -> Initialize() IsTreeEvent latch
-- (zone_azuremyst_isle.cpp:212-216: GetAreaId() == AREA_COVE (3579)
-- or AREA_ISLE (3639)), latched at construct and at every Reset.
local treeEvent = {}

-- C++ Initialize() latch; event 23 fires on spawn, so the onReset
-- capture below equals the C++ constructor capture. The helper
-- recomputes live on a cache miss — a stationary NPC never changes
-- areas mid-life, so this equals the latch.
local function isTreeEvent(creature, guid)
    local latched = treeEvent[guid]
    if latched == nil then
        latched = (creature:GetAreaId() == 3579 or creature:GetAreaId() == 3639)
        treeEvent[guid] = latched
    end
    return latched
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

local function onDynamite(creature, guid)
    local victim = creature:GetVictim()
    if victim then
        creature:CastSpell(victim, SPELL_DYNAMITE)
    end
    schedule(guid, "dynamite", DYNAMITE_DELAY_MS, function()
        onDynamite(creature, guid)
    end)
end

local function onEmote(creature, guid)
    if not creature:GetVictim() then
        creature:Talk(SAY_TEXT)
        creature:Talk(SAY_EMOTE)
    end
    schedule(guid, "emote", math.random(EMOTE_MIN_MS, EMOTE_MAX_MS), function()
        onEmote(creature, guid)
    end)
end

local function onEnterCombat(event, creature, target)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    creature:Talk(ATTACK_YELL, target)
    -- C++ UpdateAI returns before the dynamite pump when IsTreeEvent
    -- (zone_azuremyst_isle.cpp:251-252): the tree-event Spark never
    -- casts. JustEngagedWith's Talk above is unconditional in C++
    -- (:227-230), so it stays outside the gate.
    if not isTreeEvent(creature, guid) then
        schedule(guid, "dynamite", DYNAMITE_DELAY_MS, function()
            onDynamite(creature, guid)
        end)
    end
end

local function onLeaveCombat(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- C++ emote pump runs only when !IsInCombat() && !IsTreeEvent
    -- (zone_azuremyst_isle.cpp:242-249): the tree-event Spark never
    -- emotes either.
    if not isTreeEvent(creature, guid) then
        schedule(guid, "emote", math.random(EMOTE_MIN_MS, EMOTE_MAX_MS), function()
            onEmote(creature, guid)
        end)
    end
end

-- C++ has no JustDied arm, but a dead creature stops UpdateAI, so the
-- re-arming dynamite / emote timers must be cancelled on death or they
-- keep casting from the corpse forever. Event 23 fires before 4 on the
-- kill path and its IsDead guard skips the emote re-arm, so this lands
-- the same reset as leave-combat without re-arming anything.
local function onDied(event, creature)
    cancelTimers(creature:GetGUIDLow())
end

local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    -- zone_azuremyst_isle.cpp:223-224 — Reset restores the
    -- constructor-captured faction + UNIT_NPC_FLAGS via SetFaction /
    -- SetUInt32Value, C++ order after Initialize().
    -- creature:SetFaction (lua_creature_events.go:6712);
    -- creature:SetNPCFlags (lua_objects.go:3274). Event 23 fires
    -- on spawn/respawn (:7989), evade (:3475-3477,
    -- creaturemotion.go:4733) and the kill path (:7857-7859,
    -- :7906-7908), so the capture lands at first reset — equal to
    -- the C++ constructor capture, since nothing mutates this
    -- creature's faction/flags before then.
    if normFaction[guid] == nil then
        normFaction[guid] = creature:GetFaction()
    end
    creature:SetFaction(normFaction[guid])
    if npcFlags[guid] == nil then
        npcFlags[guid] = creature:GetNPCFlags()
    end
    creature:SetNPCFlags(npcFlags[guid])
    -- C++ Reset -> Initialize() re-latches IsTreeEvent (:213-216, :221).
    treeEvent[guid] = (creature:GetAreaId() == 3579 or creature:GetAreaId() == 3639)
    -- C++ UpdateAI never runs while dead (zone_azuremyst_isle.cpp:240-264);
    -- a death-reset (23 fires before 4 on the kill path) must not re-arm
    -- the emote timer, or the corpse Talks every 120-150s until respawn.
    -- The emote re-arm is also IsTreeEvent-gated (:242): the tree-event
    -- Spark never emotes.
    if not creature:IsDead() and not treeEvent[guid] then
        schedule(guid, "emote", math.random(EMOTE_MIN_MS, EMOTE_MAX_MS), function()
            onEmote(creature, guid)
        end)
    end
end

RegisterCreatureEvent(ENTRY, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY, 2, onLeaveCombat)
RegisterCreatureEvent(ENTRY, 4, onDied)
RegisterCreatureEvent(ENTRY, 23, onReset)

-- C++ OnGossipSelect (zone_azuremyst_isle.cpp:232-239):
-- CloseGossipMenuFor + SetFaction(FACTION_MONSTER) +
-- Attack(player, true); returns false. player:GossipComplete()
-- is the CloseGossipMenuFor twin (depth_charge rizzle
-- precedent); creature:Attack(player, true) is the arlokk
-- precedent (lua_creature_events.go:1945, Attack(target[,
-- meleeAttack])).
local function onGossipSelect(event, player, creature)
    player:GossipComplete()
    creature:SetFaction(FACTION_MONSTER)
    creature:Attack(player, true)
    return false
end

RegisterCreatureGossipEvent(ENTRY, 2, onGossipSelect)
