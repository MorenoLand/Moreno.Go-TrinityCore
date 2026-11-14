-- Nagrand maghar/kurenai captives (Outland zone script) --
-- Lua port of src/server/scripts/Outland/zone_nagrand.cpp
-- (npc_maghar_captive / npc_kurenai_captive — the two
-- CreatureScript EscortAIs AddSC_nagrand registers).
-- Entries (TDB 335.26091 creature_template ScriptName):
-- 18210 Mag'har Captive (npc_maghar_captive), 18209 Kurenai
-- Captive (npc_kurenai_captive). Quests: 9868 (Horde,
-- "Totem of Kar'dash" chain) / 9879 (Alliance).
-- Eluna creature events: 31 OnQuestAccept, 6 OnReachWP, 19
-- OnJustSummonedCreature, 15 OnSpellHitTarget, 1
-- OnEnterCombat, 2 OnLeaveCombat, 4 OnDied, 23 OnReset.
-- Ported arms (C++-exact, zone_nagrand.cpp:43-227 maghar,
-- :231-419 kurenai):
-- - OnQuestAccept (31): quest id match -> SetStandState(0
--   STAND) + SetFaction(232 maghar / 231 kurenai,
--   FACTION_ESCORTEE_H/A_NEUTRAL_ACTIVE) + StartEscort(player)
--   + MoveWaypoint() (the C++ Start(true, false, ...) walk) +
--   Talk(START 0) + 3x ambush-A murk summons (type 3
--   TEMPSUMMON_TIMED_DESPAWN_OUT_OF_COMBAT, 25000ms, C++
--   offsets verbatim: maghar raider +2.5/-2.5, putrifier
--   -2.5/+2.5, brute 0/0; kurenai raider +2.5/-2.5, brute
--   -2.5/+2.5, scavenger 0/0).
-- - JustDied (4): IsEscorted() + GetEscortPlayer() nil-guard
--   (the HasEscortState(ESCORTING) twin) + quest status !=
--   COMPLETE -> player:FailQuest(quest).
-- - JustEngagedWith (1): non-triggered DoCast(me,
--   SPELL_EARTHBIND_TOTEM 15786) + combat pump start with the
--   Initialize() latches (chain 1000, heal 0, frost 6000).
-- - UpdateAI combat pump (1s CreateLuaEvent, C++ arm order):
--   IsCasting gate (the UNIT_STATE_CASTING return); chain
--   lightning DoCastVictim(16006), re-arm urand(7000,14000);
--   HealthBelowPct(30) via GetHealthPct() <= 30 -> self
--   HealingWave(12491), 5000 re-arm (HealTimer freezes above
--   30%, wilda heal-pump shape; first sub-30% tick casts
--   immediately, timer starts at 0); frost shock
--   DoCastVictim(12548), re-arm urand(7500,15000). Melee is
--   engine-driven. Pump runs only between enter-combat and
--   leave-combat/died/reset (the !UpdateVictim gate).
-- - SpellHitTarget (15, caster-side; NOT event 14, which is
--   the victim-side OnHitBySpell — the banner file's "event
--   14" label was the wrong number): maghar chain lightning
--   rand32()%10==0 -> Talk(LIGHTNING 2); kurenai chain
--   lightning rand32()%30==0 -> Talk(4), frost shock
--   rand32()%30==0 -> Talk(SHOCK 5). Maghar has NO shock arm.
-- - JustSummoned (19): brute entry 18211 -> maghar
--   summoned:Talk(NO_ESCAPE 0), kurenai creature:Talk(1) on
--   ME (different receivers, C++-verbatim); totem entry 2630
--   (Earthbind Totem) -> return (no IsTotem bridge; the
--   entry check is the twin); else summoned:SetWalk(false) +
--   summoned:MoveTo(0, captive X/Y/Z via the generic
--   GetX/GetY/GetZ fallback, scripting/object.go:196) +
--   summoned:AttackStart(captive) (the C++ MovePoint(0, me
--   pos) + AI()->AttackStart(me) pair).
-- - WaypointReached (6, DB point via GetCurrentWaypointId):
--   maghar wp7 Talk(MORE 1) + 4x ambush-B summons (first
--   putrifier Talks MORE_REPLY 0 on itself if non-nil) +
--   wp16 Talk(COMPLETE 4) + GroupEventHappens(9868) +
--   SetRun() (twin: SetWalk(false)); kurenai wp3 Talk(MORE 2)
--   + putrifier at ambush B, Talk(MORE_TWO 3) on ME if
--   non-nil, + 3 more summons + wp7 Talk(COMPLETE 6) +
--   GroupEventHappens(9879) + SetWalk(false). Ambush summons
--   spawn at the FIXED ambush-B coords, not at the waypoint.
-- - Reset (23): pump cancel + state drop; the re-latch
--   lands on OnEnterCombat (gargolmar precedent).
-- Deliberate deviations / blockers (all data-side, no engine
-- work in this file):
-- - THE ESCORT WALK cannot fire against TDB 26091: C++
--   reads the path from `script_waypoint` (entry-keyed);
--   Go has no script_waypoint loader and its waypoint_data
--   loader resolves the captives to spawn GUIDs 65108/84459,
--   which have no rows; the real paths sit orphaned in
--   waypoint_data under 145674 (kurenai pts 1-10, pt7 delay
--   7500) / 145682 (maghar pts 0-19, pt7 delay 2000, pt16
--   delay 7500, pts 16-19 run) with no binding row
--   (creature_template_addon path_id = 0 for both entries).
--   MoveWaypoint() is a provable no-op here, so the ev6 arms
--   above are registered but inert until the engine loads
--   script_waypoint, gains an entry->path binding, or the
--   deployment DB gains waypoint_data rows under the spawn
--   GUIDs / a creature_addon.path_id row (wilda precedent:
--   same no-op rule, arms registered anyway). The walk loop
--   itself (Go NextIdx wraps; no end-of-path idle twin) and
--   the combat-pause question (Go's waypoint launch gate has
--   EscortPaused / casting gates but NO engaged/victim gate,
--   creaturemotion.go:9801-9830) are moot until the walk can
--   start.
-- - Talk is silent against TDB 26091: the dump's
--   creature_text table holds exactly two rows (entries 6
--   and 23383) — NO rows for 18209/18210, so every Talk(id)
--   here resolves to nothing. CORRECTION of the captive
--   re-triage report, which claimed groups 0-4 rows exist
--   (it read the npc_text gossip rows for these entries
--   instead). Talk ids are ported C++-verbatim (kurenai
--   groups 0-6 as written, incl. 5/6 with no rows anywhere);
--   they fire once the deployment DB gains the rows.
-- - Totem ev19 gate is moot in Go: totems.go summonTotem
--   fires no JustSummoned event, so the totem never reaches
--   ev19; the entry-2630 guard stays as C++-faithful defense.
-- - C++ Start() while-engaged / already-escorting error
--   returns have no Go twin (StartEscort only arms the GUID);
--   a re-accept guard is a Lua-side latch (none needed: the
--   quest can only be accepted once per player).

local ENTRY_MAGHAR_CAPTIVE = 18210
local ENTRY_KURENAI_CAPTIVE = 18209

local SPELL_CHAIN_LIGHTNING = 16006
local SPELL_EARTHBIND_TOTEM = 15786
local SPELL_FROST_SHOCK = 12548
local SPELL_HEALING_WAVE = 12491

local NPC_MURK_RAIDER = 18203
local NPC_MURK_BRUTE = 18211
local NPC_MURK_SCAVENGER = 18207
local NPC_MURK_PUTRIFIER = 18202
local NPC_EARTHBIND_TOTEM = 2630

local QUEST_STATUS_COMPLETE = 1
local STAND_STATE_STAND = 0

local SUMMON_TYPE_OUT_OF_COMBAT = 3
local SUMMON_DESPAWN_MS = 25000

-- Ambush A/B, zone_nagrand.cpp:67-68 / :256-257 (shared by
-- both captives: kurenaiAmbushA/B repeat the same coords).
local AMBUSH_A_X, AMBUSH_A_Y, AMBUSH_A_Z = -1568.805786, 8533.873047, 1.958
local AMBUSH_B_X, AMBUSH_B_Y, AMBUSH_B_Z = -1491.554321, 8506.483398, 1.248

-- Per-captive config: the two AIs differ only in quest,
-- faction, Talk groups, ambush-A summon set, waypoint ids,
-- and the SpellHitTarget gates / Talk receivers.
local configs = {
    [ENTRY_MAGHAR_CAPTIVE] = {
        quest = 9868,
        faction = 232,
        sayStart = 0,
        sayNoEscape = 0,
        sayMore = 1,
        sayMoreReply = 0,
        sayLightning = 2,
        sayComplete = 4,
        lightningMod = 10,
        hasShockArm = false,
        ambushWp = 7,
        completeWp = 16,
        -- Ambush-A summon set, C++ order: {entry, dx, dy}.
        ambushA = {
            { NPC_MURK_RAIDER, 2.5, -2.5 },
            { NPC_MURK_PUTRIFIER, -2.5, 2.5 },
            { NPC_MURK_BRUTE, 0, 0 },
        },
        -- The brute Talk lands on the SUMMONED brute.
        bruteTalkOnSummoned = true,
    },
    [ENTRY_KURENAI_CAPTIVE] = {
        quest = 9879,
        faction = 231,
        sayStart = 0,
        sayNoEscape = 1,
        sayMore = 2,
        sayMoreTwo = 3,
        sayLightning = 4,
        sayShock = 5,
        sayComplete = 6,
        lightningMod = 30,
        shockMod = 30,
        hasShockArm = true,
        ambushWp = 3,
        completeWp = 7,
        ambushA = {
            { NPC_MURK_RAIDER, 2.5, -2.5 },
            { NPC_MURK_BRUTE, -2.5, 2.5 },
            { NPC_MURK_SCAVENGER, 0, 0 },
        },
        -- The brute Talk lands on ME (the captive).
        bruteTalkOnSummoned = false,
    },
}

local timers = {}
local states = {}

local function cancelPump(guid)
    local id = timers[guid]
    if id then
        RemoveEventById(id)
        timers[guid] = nil
    end
end

local function fullReset(guid)
    cancelPump(guid)
    states[guid] = nil
end

-- C++ UpdateAI (zone_nagrand.cpp:164-201 / :364-397) in arm
-- order — 1s granularity exact for all C++ timers.
local function captiveTick(creature, guid)
    local st = states[guid]
    if not st then
        return
    end
    -- C++ HasUnitState(UNIT_STATE_CASTING) early return.
    if creature:IsCasting() then
        return
    end
    -- Chain lightning arm: 1s init then 7-14s, non-triggered
    -- DoCastVictim (C++-exact).
    if st.chainLightning <= 1000 then
        creature:CastSpell(nil, SPELL_CHAIN_LIGHTNING)
        st.chainLightning = 7000 + math.random(0, 7000)
    else
        st.chainLightning = st.chainLightning - 1000
    end
    -- Healing-wave arm: only while at-or-below 30% (the
    -- HealTimer freezes above 30%); 0s init so the first
    -- sub-30% tick casts immediately, then 5s re-arm.
    if creature:GetHealthPct() <= 30 then
        if st.heal <= 1000 then
            creature:CastSpell(creature, SPELL_HEALING_WAVE)
            st.heal = 5000
        else
            st.heal = st.heal - 1000
        end
    end
    -- Frost shock arm: 6s init then 7.5-15s, non-triggered
    -- DoCastVictim (C++-exact).
    if st.frostShock <= 1000 then
        creature:CastSpell(nil, SPELL_FROST_SHOCK)
        st.frostShock = 7500 + math.random(0, 7500)
    else
        st.frostShock = st.frostShock - 1000
    end
end

local function startPump(guid, creature)
    cancelPump(guid)
    timers[guid] = CreateLuaEvent(function()
        captiveTick(creature, guid)
    end, 1000, 0)
end

-- C++ OnQuestAccept (zone_nagrand.cpp:203-219 / :400-412),
-- C++ order: SetStandState, SetFaction, Start, Talk, summons.
local function onQuestAccept(event, player, creature, quest)
    local cfg = configs[creature:GetEntry()]
    if not cfg or not quest or quest:GetId() ~= cfg.quest then
        return
    end
    creature:SetStandState(STAND_STATE_STAND)
    creature:SetFaction(cfg.faction)
    creature:StartEscort(player)
    creature:MoveWaypoint()
    creature:Talk(cfg.sayStart)
    for _, summon in ipairs(cfg.ambushA) do
        creature:SummonCreature(summon[1],
            AMBUSH_A_X + summon[2], AMBUSH_A_Y + summon[3], AMBUSH_A_Z,
            0, SUMMON_TYPE_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
    end
end

-- C++ WaypointReached (zone_nagrand.cpp:114-137 / :302-329).
-- The ambush legs use no escort player; only the credit leg
-- nil-guards GetPlayerForEscort, C++-verbatim.
local function onReachWP(event, creature)
    local cfg = configs[creature:GetEntry()]
    if not cfg then
        return
    end
    local wp = creature:GetCurrentWaypointId()
    if wp == cfg.ambushWp then
        creature:Talk(cfg.sayMore)
        local first = creature:SummonCreature(NPC_MURK_PUTRIFIER,
            AMBUSH_B_X, AMBUSH_B_Y, AMBUSH_B_Z,
            0, SUMMON_TYPE_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
        if first then
            if cfg.bruteTalkOnSummoned then
                -- Maghar: the first putrifier Talks MORE_REPLY.
                first:Talk(cfg.sayMoreReply)
            else
                -- Kurenai: ME Talks MORE_TWO if the summon landed.
                creature:Talk(cfg.sayMoreTwo)
            end
        end
        creature:SummonCreature(NPC_MURK_PUTRIFIER,
            AMBUSH_B_X - 2.5, AMBUSH_B_Y - 2.5, AMBUSH_B_Z,
            0, SUMMON_TYPE_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
        creature:SummonCreature(NPC_MURK_SCAVENGER,
            AMBUSH_B_X + 2.5, AMBUSH_B_Y + 2.5, AMBUSH_B_Z,
            0, SUMMON_TYPE_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
        creature:SummonCreature(NPC_MURK_SCAVENGER,
            AMBUSH_B_X + 2.5, AMBUSH_B_Y - 2.5, AMBUSH_B_Z,
            0, SUMMON_TYPE_OUT_OF_COMBAT, SUMMON_DESPAWN_MS)
    elseif wp == cfg.completeWp then
        creature:Talk(cfg.sayComplete)
        local player = creature:GetEscortPlayer()
        if player then
            player:GroupEventHappens(cfg.quest, creature)
        end
        -- C++ SetRun(); the SetWalk(false) twin (old_hillsbrad).
        creature:SetWalk(false)
    end
end

-- C++ JustSummoned (zone_nagrand.cpp:139-150 / :331-343),
-- C++ order: brute Talk, totem return, walk/move/attack.
local function onJustSummoned(event, creature, summoned)
    if not summoned then
        return
    end
    local cfg = configs[creature:GetEntry()]
    if not cfg then
        return
    end
    if summoned:GetEntry() == NPC_MURK_BRUTE then
        if cfg.bruteTalkOnSummoned then
            summoned:Talk(cfg.sayNoEscape)
        else
            creature:Talk(cfg.sayNoEscape)
        end
    end
    if summoned:GetEntry() == NPC_EARTHBIND_TOTEM then
        return
    end
    summoned:SetWalk(false)
    summoned:MoveTo(0, creature:GetX(), creature:GetY(), creature:GetZ())
    summoned:AttackStart(creature)
end

-- C++ SpellHitTarget (zone_nagrand.cpp:153-162 / :345-362).
-- Handler args (event, creature, target, spellId), eredar
-- twins precedent. rand32()%N==0 -> Talk (1/N chance).
local function onSpellHitTarget(event, creature, target, spellId)
    local cfg = configs[creature:GetEntry()]
    if not cfg then
        return
    end
    if spellId == SPELL_CHAIN_LIGHTNING then
        if math.random(0, cfg.lightningMod - 1) == 0 then
            creature:Talk(cfg.sayLightning)
        end
    elseif cfg.hasShockArm and spellId == SPELL_FROST_SHOCK then
        if math.random(0, cfg.shockMod - 1) == 0 then
            creature:Talk(cfg.sayShock)
        end
    end
end

-- C++ JustEngagedWith (zone_nagrand.cpp:94-100 / :283-289):
-- earthbind totem + pump start with Initialize() latches.
local function onEnterCombat(event, creature)
    local guid = creature:GetGUIDLow()
    states[guid] = {
        chainLightning = 1000,
        heal = 0,
        frostShock = 6000,
    }
    creature:CastSpell(creature, SPELL_EARTHBIND_TOTEM)
    startPump(guid, creature)
end

-- C++ JustDied (zone_nagrand.cpp:102-112 / :291-301): escort
-- + quest-incomplete -> FailQuest; then the pump/state drop.
local function onDied(event, creature)
    local cfg = configs[creature:GetEntry()]
    if cfg and creature:IsEscorted() then
        local player = creature:GetEscortPlayer()
        if player and player:GetQuestStatus(cfg.quest) ~= QUEST_STATUS_COMPLETE then
            player:FailQuest(cfg.quest)
        end
    end
    fullReset(creature:GetGUIDLow())
end

local captiveEntries = { ENTRY_MAGHAR_CAPTIVE, ENTRY_KURENAI_CAPTIVE }
for _, entry in ipairs(captiveEntries) do
    RegisterCreatureEvent(entry, 31, onQuestAccept)
    RegisterCreatureEvent(entry, 6, onReachWP)
    RegisterCreatureEvent(entry, 19, onJustSummoned)
    RegisterCreatureEvent(entry, 15, onSpellHitTarget)
    RegisterCreatureEvent(entry, 1, onEnterCombat)
    RegisterCreatureEvent(entry, 2, function(_, creature)
        fullReset(creature:GetGUIDLow())
    end)
    RegisterCreatureEvent(entry, 4, onDied)
    RegisterCreatureEvent(entry, 23, function(_, creature)
        fullReset(creature:GetGUIDLow())
    end)
end
