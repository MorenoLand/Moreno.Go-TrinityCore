-- Beryl Sorcerer (Borean Tundra) -- Lua port of
-- src/server/scripts/Northrend/zone_borean_tundra.cpp
-- (class npc_beryl_sorcerer : CreatureScript("npc_beryl_sorcerer");
-- struct npc_beryl_sorcererAI : public FollowerAI).
-- Entry: 25316 'Beryl Sorcerer' (12 static spawns, TDB 26091;
-- ScriptName empty in this DB — the nerubar-victim / imprisoned-
-- sorcerer precedent: RegisterLuaBoss binds by entry). After the
-- enslavement the creature becomes 25474 'Captured Beryl
-- Sorcerer'; handlers register for BOTH entries (the UpdateEntry
-- bridge comment: Go dispatches on the live motion.Entry, so
-- entry-persistent handlers must cover both — nether-drake
-- precedent, same script name registered twice in boss_ai.go).
-- Eluna creature events: 5 OnSpawn, 23 OnReset, 14 OnHitBySpell,
-- 4 OnDied (timer hygiene only — C++ has no JustDied override).
-- Ported arms (C++ order):
-- - Reset (zone_borean_tundra.cpp:1112-1116): SetReactState
--   (REACT_AGGRESSIVE 2) + bEnslaved = false. The Donathan poll
--   (re)arms on every live spawn/reset — C++ MoveInLineOfSight
--   runs from spawn, ungated on bEnslaved (see below).
-- - SpellHit (zone_borean_tundra.cpp:1124-1142): player-caster
--   gate; spell 45611 SPELL_ARCANE_CHAINS + !HealthAbovePct(50)
--   (GetHealthPct() > 50 return — the exact twin: HealthAbovePct
--   is strict) + !bEnslaved -> EnterEvadeMode (fires ev23
--   synchronously in Go, so the Reset leg above runs mid-arm and
--   is then re-applied by the lines that follow, C++ order
--   preserved) + SetReactState(REACT_PASSIVE 0) + StartFollow
--   (player) + UpdateEntry(25474) + DoCast(45631 triggered) +
--   KilledMonsterCredit(25474) + bEnslaved = true. All bridges
--   live (EnterEvadeMode :5983, StartFollow :2945, UpdateEntry
--   :5572, SetReactState / KilledMonsterCredit precedents); the
--   round-45 "STAY OUT on missing StartFollow/DisappearAndDie"
--   verdict is stale on every leg it named.
-- - MoveInLineOfSight Donathan leg (zone_borean_tundra.cpp:
--   1144-1153): FollowerAI::MoveInLineOfSight + Donathan (25262)
--   within INTERACTION_DISTANCE 5.0 -> SetFollowComplete() +
--   DisappearAndDie(). The ev27 twin is the kerlonian 1s pump:
--   ev27 fires only from the aggro-engage branch and Donathan is
--   not an enemy, so the poll checks GetNearestCreature(5.0,
--   25262, 0, 1) every 1s; on a hit it sets COMPLETE and despawns
--   inline (creature:Despawn is the DisappearAndDie twin — the
--   beryl C++ does it inline in MIOS, unlike kerlonian's
--   watchdog deferral). QUIRK, C++-verbatim: the leg is ungated
--   on bEnslaved, so a wild sorcerer also despawns if Donathan
--   wanders within 5y — that is what the C++ does.
-- Unmodeled (documented-only, no bridges):
-- - FollowerAI::MoveInLineOfSight assist gate
--   (ShouldAssistPlayerInCombatAgainst): no CAN_ASSIST twin;
--   the port never assists, matching the kerlonian poll shape.
-- - JustEngagedWith (zone_borean_tundra.cpp:1118-1122): engine
--   melee (AttackStart on a valid target) — no Lua arm needed.
-- - UpdateAI (zone_borean_tundra.cpp:1155-1161): melee only.

local ENTRY_BERYL_SORCERER = 25316
local ENTRY_CAPTURED_BERYL_SORCERER = 25474

local NPC_LIBRARIAN_DONATHAN = 25262

local SPELL_ARCANE_CHAINS = 45611
local SPELL_COSMETIC_ENSLAVE_CHAINS_SELF = 45631

local REACT_PASSIVE = 0
local REACT_AGGRESSIVE = 2

local STATE_FOLLOW_INPROGRESS = 0x001

local INTERACTION_DISTANCE = 5.0
local POLL_INTERVAL_MS = 1000

local enslaved = {}
local timers = {}

-- Forward declaration: armPoll's closure calls berylTick, which is
-- assigned below. A later `local function` here would shadow this
-- slot — plain assignment only (tharon_ja/gal_darah idiom).
local berylTick

local function cancelPoll(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function armPoll(guid, creature)
    cancelPoll(guid)
    timers[guid] = CreateLuaEvent(function()
        timers[guid] = nil
        berylTick(creature, guid)
    end, POLL_INTERVAL_MS)
end

-- C++ npc_beryl_sorcererAI::MoveInLineOfSight Donathan leg
-- (zone_borean_tundra.cpp:1144-1153), 1s poll twin (kerlonian
-- idiom): Donathan within 5.0y -> SetFollowComplete + Despawn.
-- Runs out of combat and while engaged alike — C++ MIOS is not
-- combat-gated. The C++ leg is ungated on bEnslaved, so the poll
-- is armed from spawn, not only after enslavement.
berylTick = function(creature, guid)
    if creature:IsDead() then
        enslaved[guid] = nil
        return
    end
    local donathan = creature:GetNearestCreature(
        INTERACTION_DISTANCE, NPC_LIBRARIAN_DONATHAN, 0, 1)
    if donathan ~= nil then
        enslaved[guid] = nil
        creature:SetFollowComplete()
        creature:Despawn()
        return
    end
    armPoll(guid, creature)
end

-- C++ npc_beryl_sorcererAI::Reset (zone_borean_tundra.cpp:
-- 1112-1116) + Initialize (:1104-1107): react aggressive +
-- bEnslaved = false. Go fires ev23 on spawn, evade and death;
-- the dead arm drops the state (survivor idiom) and the poll
-- arms only on a live creature. The follow state survives evade
-- in Go (kerlonian idiom), so the poll re-arms after an evade
-- exactly as C++ MIOS keeps firing.
local function onReset(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPoll(guid)
    if creature:IsDead() then
        enslaved[guid] = nil
        return
    end
    creature:SetReactState(REACT_AGGRESSIVE)
    enslaved[guid] = false
    armPoll(guid, creature)
end

-- C++ npc_beryl_sorcererAI::SpellHit (zone_borean_tundra.cpp:
-- 1124-1142). Caster must be a player (caster->ToPlayer(), :1126)
-- via the caster:GetObjectType() == "Player" convention
-- (nalorakk / imprisoned-sorcerer precedent).
local function onSpellHit(event, creature, caster, spellId)
    if spellId ~= SPELL_ARCANE_CHAINS then
        return
    end
    if not caster or caster:GetObjectType() ~= "Player" then
        return
    end
    if creature:GetHealthPct() > 50 then
        return
    end
    local guid = creature:GetGUIDLow()
    if enslaved[guid] then
        return
    end
    -- C++ order: EnterEvadeMode, SetReactState(PASSIVE),
    -- StartFollow, UpdateEntry, DoCast, KilledMonsterCredit,
    -- bEnslaved = true. EnterEvadeMode fires ev23 synchronously
    -- in Go (nether-drake precedent): onReset above runs mid-arm
    -- (react aggressive + latch false + poll re-arm) and the lines
    -- that follow re-apply the enslaved state.
    creature:EnterEvadeMode()
    creature:SetReactState(REACT_PASSIVE)
    creature:StartFollow(caster)
    creature:UpdateEntry(ENTRY_CAPTURED_BERYL_SORCERER)
    creature:CastSpell(creature, SPELL_COSMETIC_ENSLAVE_CHAINS_SELF, true)
    caster:KilledMonsterCredit(ENTRY_CAPTURED_BERYL_SORCERER)
    enslaved[guid] = true
    armPoll(guid, creature)
end

-- Timer hygiene only: C++ has no JustDied override for this AI.
-- Registered on both entries so the poll dies with the creature
-- whichever entry it holds.
local function onDied(event, creature)
    local guid = creature:GetGUIDLow()
    cancelPoll(guid)
    enslaved[guid] = nil
end

RegisterCreatureEvent(ENTRY_BERYL_SORCERER, 5, onReset)
RegisterCreatureEvent(ENTRY_BERYL_SORCERER, 23, onReset)
RegisterCreatureEvent(ENTRY_BERYL_SORCERER, 14, onSpellHit)
RegisterCreatureEvent(ENTRY_BERYL_SORCERER, 4, onDied)
RegisterCreatureEvent(ENTRY_CAPTURED_BERYL_SORCERER, 5, onReset)
RegisterCreatureEvent(ENTRY_CAPTURED_BERYL_SORCERER, 23, onReset)
RegisterCreatureEvent(ENTRY_CAPTURED_BERYL_SORCERER, 14, onSpellHit)
RegisterCreatureEvent(ENTRY_CAPTURED_BERYL_SORCERER, 4, onDied)
