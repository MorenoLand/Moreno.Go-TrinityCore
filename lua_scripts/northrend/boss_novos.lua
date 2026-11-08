-- Novos the Summoner (Drak'Tharon Keep) — Lua port of
-- src/server/scripts/Northrend/DraktharonKeep/boss_novos.cpp
-- (boss_novos (BossAI), npc_crystal_channel_target (ScriptedAI),
-- spell_novos_summon_minions (SpellScript 59910),
-- achievement_oh_novos (AchievementCriteriaScript);
-- AddSC_boss_novos at end registers all via GetDrakTharonKeepAI /
-- new; npc_crystal_channel_target gets no instance shim — it is
-- GetDrakTharonKeepAI'd like the boss). The second Drak'Tharon Keep
-- group in northrend_script_loader.cpp order (decl 40 / call 235,
-- immediately after AddSC_boss_trollgore(); next:
-- AddSC_boss_king_dred()). Entry: 26631 Novos the Summoner
-- (drak_tharon_keep.h NPC_NOVOS — kalecgos pass; the
-- GetDrakTharonKeepAI ScriptName binding is instance-shimmed, the
-- creature_template binding DB-side as usual).
-- Sole-source verified: whole-server-tree grep for "boss_novos",
-- "npc_crystal_channel_target", "spell_novos_summon_minions" and
-- "achievement_oh_novos" hits boss_novos.cpp only (the loader carries
-- only the AddSC_boss_novos decl/call lines); zero sql/ hits.
-- Eluna creature events: 1 OnEnterCombat, 3 OnTargetDied, 4 OnDied.
-- No timers are scheduled: every C++ scheduled arm rides an absent
-- bridge (see below), so the cancel/schedule machinery has nothing to
-- drive. Novos never melees in C++ (boss_novos.cpp:140 UpdateAI has
-- no DoMeleeAttackIfReady arm) — creature:SetMeleeAttack(false) at event 1
-- suppresses the engine-driven swing. BossAI _Reset/_JustDied instance
-- bookkeeping has no bridge.
-- Ported arms (C++-exact for the modeled arms — the three wired
-- hooks fire unconditionally in C++ and are independent of the
-- unmodeled bubbled-phase machinery, which gates UpdateAI entirely
-- via a latch with no bridge):
-- boss_novos: JustEngagedWith Talk(SAY_AGGRO 0) (event 1 — the hook
-- itself is bridged; its other legs — SetCrystalsStatus(true) /
-- SetSummonerStatus(true) / SetBubbled(true) (instance GetGuidData +
-- ObjectAccessor cross-AI + flag/GO-state bridges absent; instance
-- model absent — standing) — documented-only below);
-- KilledUnit Talk(SAY_KILL 1) player-gated (event 3,
-- who->GetTypeId() == TYPEID_PLAYER — nalorakk precedent);
-- JustDied Talk(SAY_DEATH 2) (event 4; _JustDied instance
-- bookkeeping has no bridge).
-- AttackStart — DoStartNoMovement (novos never moves) — PORTED
-- 2026-10-22 via creature:SetCombatMovement(false) at OnEnterCombat
-- (lua_creature_events.go:759; per-tick gate holds all fight).
-- Unmodeled (no bridges — documented, not wired):
-- Reset's SetCrystalsStatus(false) / SetSummonerStatus(false) /
-- SetBubbled(false) legs — instance GetGuidData + ObjectAccessor
-- cross-AI bridges absent; instance model absent (standing).
-- SetBubbled(true)'s DoCast(SPELL_ARCANE_FIELD 47346) + UNIT_FLAG_
-- NON_ATTACKABLE flag set — field-59 HasFlag/SetFlag/RemoveFlag ARE
-- bridged (obsidian :358 precedent; drakkari_colossus claim retracted);
-- the _bubbled latch gates UpdateAI entirely while set
-- and clears only via the unported instance-script DoAction sender
-- (instance_drak_tharon_keep.cpp:153), so no latch is emulated
-- (jedoga precedent — an ungated bubbled model would diverge from
-- C++ combat behavior).
-- UpdateAI: the !_bubbled gate (see above) + HasUnitState(
-- UNIT_STATE_CASTING) pre-loop (:147) + in-loop (:167) gates guard
-- only the unported EVENT_ATTACK / EVENT_SUMMON_MINIONS arms (both
-- schedule only after the unported instance-script DoAction sender
-- (instance_drak_tharon_keep.cpp:153) clears the bubble); ported
-- Talk arms ungated per C++ (outside
-- UpdateAI). Stale "constant-false IsCasting" claim retracted —
-- IsCasting is live (lua_creature_events.go:1950).
-- EVENT_ATTACK (3s repeat, scheduled only after the crystal-handler
-- DoAction machine clears the bubble) — SelectTarget(Random) ->
-- DoCast(victim, RAND(SPELL_ARCANE_BLAST 49198 / SPELL_BLIZZARD
-- 49034 / SPELL_FROSTBOLT 49037 / SPELL_WRATH_OF_MISERY 50089)) —
-- random-target SelectTarget bridge absent (cairne/kazzak precedent).
-- EVENT_SUMMON_MINIONS (15s repeat, heroic-only, likewise
-- DoAction-gated) — DoCast(SPELL_SUMMON_MINIONS 59910) — would
-- over-cast ungated (jedoga precedent) — documented-only.
-- The DoAction machine — DoAction(ACTION_CRYSTAL_HANDLER_DIED)
-- (drak_tharon_keep.h:52 = 15) + CrystalHandlerDied: per-crystal
-- SetCrystalStatus(crystal, false) deactivation (instance GetGuidData
-- + ObjectAccessor + GO-state bridges absent) stays out; the
-- _crystalHandlerCount latch -> Talk(SAY_ARCANE_FIELD 4) + the
-- SetBubbled(false) RemoveFlag/CastStop legs are PORTED 2026-11-08
-- (tick 466) via the ev38 DoAction handler below, driven by the
-- instance_drak_tharon_keep.lua OnUnitDeath sender (Go-only
-- INSTANCE_EVENT_ON_UNIT_DEATH 9); the count zeroes on Reset (ev23).
-- SetSummonerStatus(false), the EVENT_ATTACK / EVENT_SUMMON_MINIONS
-- schedules, and the per-handler-death AI()->SetData(
-- SPELL_SUMMON_CRYSTAL_HANDLER, 15000) re-arm on the
-- DATA_NOVOS_SUMMONER_4 target stay out — cross-AI SetData and the
-- Novos UpdateAI timers are absent.
-- MoveInLineOfSight — the _ohNovos latch (Initialize/Reset true;
-- flips false when a HULKING_CORPSE 27597 / RISEN_SHADOWCASTER 27600
-- / FETID_TROLL_CORPSE 27598 enters LOS at y <= -771.95f — C++
-- returns early above MAX_Y_COORD_OH_NOVOS) — PORTED 2026-11-08
-- (tick 467) as a 1s Novos-side proximity poll (ohNovosCheck below):
-- the grid MoveInLineOfSight notification has no Go bridge (ev27
-- fires only from the engage-only aggro scan, and Novos is in combat
-- while the adds approach), so Novos scans for the three entries
-- with FindNearestCreature (fresh objects — the add's live Y rides
-- the snapshot) inside the Go ev27 sighting distance (45yd, the
-- aggro-scan cap); the C++ grid visibility range is wider, so the
-- check errs toward holding the latch — the documented delta.
-- DORMANT until the unported crystal-channel-target summoner chain
-- spawns the adds (no Go path summons 27597/27598/27600 today).
-- GetData(DATA_NOVOS_ACHIEV) (type == DATA_NOVOS_ACHIEV && _ohNovos
-- ? 1 : 0) — PORTED 2026-11-08 (tick 469): the latch is mirrored into
-- the SetData store (Reset writes 1, the poll writes 0), and the Go
-- achievement_oh_novos criteria check reads it via bossLatchData off
-- the killed creature — no cross-AI GetData bridge is needed, the
-- target IS Novos. The stale "no cross-AI GetData bridge" claim is
-- retired: the only caller is achievement_oh_novos and it reads the
-- target's own AI.
-- JustSummoned — summons.Summon — summon STRAND absent (standing) —
-- documented-only.
-- npc_crystal_channel_target — ENTRY-VERIFIABLE BUT BRIDGE-BLOCKED
-- (26712 — drak_tharon_keep.h NPC_CRYSTAL_CHANNEL_TARGET, kalecgos
-- pass): zero registration. The whole AI is the cross-AI timer
-- machine: novos-side SetSummonerStatus / CrystalHandlerDied drive
-- it via AI()->SetData(spell, timer), then UpdateAI DoCast(_spell)
-- on expiry; JustSummoned forwards to novos->AI()->JustSummoned and
-- runs MovePath(summon->GetEntry() * 100) (motion-master bridge
-- absent). With the cross-AI SetData activation bridge absent it can
-- never fire — documented-only. Joins the
-- entry-verifiable-but-bridge-blocked queue.
-- spell_novos_summon_minions (59910 SpellScript:
-- OnEffectHitTarget EFFECT_0 SCRIPT_EFFECT -> caster double-cast
-- SPELL_SUMMON_COPY_OF_MINIONS 59933, triggered) — PORTED
-- 2026-11-08 (tick 463) via RegisterSpellEvent ev5 below (the
-- razelikh "no SpellScript bridge" stay-out is retired: ev5 is
-- the live SpellScript hook). DORMANT until the unported instance
-- DoAction machine clears the bubble and EVENT_SUMMON_MINIONS
-- casts 59910; 59933's own summon is a DBC effect.
-- achievement_oh_novos — AchievementCriteriaScript (target
-- ToCreature -> AI()->GetData(DATA_NOVOS_ACHIEV)) — PORTED 2026-11-08
-- (tick 469): Go case in achievement_criteria_scripts.go reads the
-- SetData mirror via bossLatchData (whole-server type-11 census this
-- tick: the last missing script name). The stale "no
-- achievement-criteria bridge (snakes precedent)" claim is retired —
-- the type-11 dispatch + bossLatchData bridge cover it.
-- NOTE: SAY_SUMMONING_ADDS (3) / EMOTE_SUMMONING_ADDS (5) are
-- marked unused in the Yells enum and never fired by any code in the
-- file — nothing to port.

local ENTRY_NOVOS = 26631

local SPELL_SUMMON_MINIONS = 59910
local SPELL_SUMMON_COPY_OF_MINIONS = 59933

local SAY_AGGRO = 0
local SAY_KILL = 1
local SAY_DEATH = 2
local SAY_ARCANE_FIELD = 4

local ACTION_CRYSTAL_HANDLER_DIED = 15

local DATA_NOVOS_ACHIEV = 5 -- boss_novos.cpp:62 (Misc enum)

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2

-- The _ohNovos latch (boss_novos.cpp:178-189, MoveInLineOfSight):
-- true until a Hulking Corpse / Risen Shadowcaster / Fetid Troll
-- Corpse comes within sight of Novos at y <= MAX_Y_COORD_OH_NOVOS.
local MAX_Y_COORD_OH_NOVOS = -771.95 -- boss_novos.cpp:78
local OH_NOVOS_SIGHT_YD = 45 -- the Go ev27 sighting distance (aggro-scan cap)
local ADD_ENTRIES = { 27597, 27600, 27598 } -- drak_tharon_keep.h:70-72
local ohNovos = true

-- Timers keyed by GUID (the kologarn CreateLuaEvent idiom):
-- cancelled on death (ev4) and Reset (ev23) like every other
-- per-creature pump in this tree.
local timers = {}

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

-- C++ boss_novos::MoveInLineOfSight (boss_novos.cpp:178-189) as a
-- 1s poll: an add of the three entries within OH_NOVOS_SIGHT_YD of
-- Novos at y <= MAX_Y_COORD_OH_NOVOS clears the latch (the C++
-- early-returns above the line and for non-unit / non-listed
-- entries are the FindNearestCreature entry filter + the Y gate).
-- FindNearestCreature returns a fresh object, so the add's GetY is
-- its live position. One-way: once false the pump stops (no re-arm).
local function ohNovosCheck(creature)
    if not ohNovos then
        return
    end
    for _, entry in ipairs(ADD_ENTRIES) do
        local add = creature:FindNearestCreature(entry, OH_NOVOS_SIGHT_YD)
        if add ~= nil and add:GetY() <= MAX_Y_COORD_OH_NOVOS then
            ohNovos = false
            -- Mirror the latch into the SetData store for the
            -- achievement_oh_novos Go criteria check (GetData twin).
            creature:SetData(DATA_NOVOS_ACHIEV, 0)
            return
        end
    end
    schedule(creature:GetGUID(), "oh_novos", 1000, function() ohNovosCheck(creature) end)
end

-- _crystalHandlerCount (boss_novos.cpp:261-276, CrystalHandlerDied):
-- counts crystal-handler deaths in this pull; Reset zeroes it
-- (Initialize, boss_novos.cpp:95-97). It is NOT zeroed at enter
-- combat — the four handlers die while Novos is still bubbled,
-- before the pull, so the count must survive up to the bubble
-- clearing at 4.
local crystalHandlerCount = 0

local function novosEnterCombat(event, creature, target)
    creature:SetMeleeAttack(false) -- boss_novos.cpp:140: custom UpdateAI never calls DoMeleeAttackIfReady (cast-only in C++)
    -- boss_novos.cpp:124-125 AttackStart — DoStartNoMovement on every
    -- AttackStart (novos never moves); modeled engage-time via the
    -- SetCombatMovement bridge (per-tick gate — flag stays false all fight).
    creature:SetCombatMovement(false)
    creature:SetCombatPulseDelay(5)
    creature:DoZoneInCombat()
    creature:Talk(SAY_AGGRO)
    -- The _ohNovos poll starts at engage: the adds only spawn once
    -- the summoners activate on JustEngagedWith.
    schedule(creature:GetGUID(), "oh_novos", 1000, function() ohNovosCheck(creature) end)
end

local function novosTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_KILL)
    end
end

local function novosDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
end

-- C++ boss_novos::Reset (boss_novos.cpp:99-106): _Reset + Initialize
-- zeroes _crystalHandlerCount. The SetCrystalsStatus(false) /
-- SetSummonerStatus(false) / SetBubbled(false) legs stay out —
-- instance GetGuidData + ObjectAccessor cross-AI + GO-state bridges
-- absent (the header's Reset note).
local function novosReset(event, creature)
    crystalHandlerCount = 0
    -- C++ boss_novos::Initialize (:95-97): _ohNovos resets true; the
    -- poll stops with the pull.
    ohNovos = true
    -- Mirror the latch into the SetData store (ev23 fires on spawn
    -- too, so the initial true is written before any kill can read it).
    creature:SetData(DATA_NOVOS_ACHIEV, 1)
    cancelTimers(creature:GetGUID())
end

-- C++ boss_novos::DoAction(ACTION_CRYSTAL_HANDLER_DIED)
-- (boss_novos.cpp:172-175) + CrystalHandlerDied (:261-276), driven by
-- the instance script OnUnitDeath sender (instance_drak_tharon_keep.lua,
-- tick 466). The per-crystal deactivation loop (first ACTIVE crystal ->
-- GO_STATE_READY + channel-target CastStop) stays out — no GO-state
-- bridge. At 4 deaths: Talk(SAY_ARCANE_FIELD 4) + SetSummonerStatus(false)
-- (cross-AI, stays out) + SetBubbled(false) (the RemoveFlag NON_ATTACKABLE
-- + CastStop legs — HasFlag/RemoveFlag and IsCasting/CastStop are
-- bridged) + the EVENT_ATTACK (3s) / EVENT_SUMMON_MINIONS (15s, heroic)
-- schedules (no Novos UpdateAI timers are ported — the UpdateAI arms
-- stay out, so the schedules have nothing to drive). Below 4: the
-- SetData(SPELL_SUMMON_CRYSTAL_HANDLER, 15000) re-arm on the
-- DATA_NOVOS_SUMMONER_4 target stays out — cross-AI SetData absent.
local function novosDoAction(event, creature, action)
    if action ~= ACTION_CRYSTAL_HANDLER_DIED then
        return
    end
    crystalHandlerCount = crystalHandlerCount + 1
    if crystalHandlerCount >= 4 then
        creature:Talk(SAY_ARCANE_FIELD)
        if creature:HasFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE) then
            creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NON_ATTACKABLE)
        end
        if creature:IsCasting() then
            creature:CastStop()
        end
    end
end

RegisterCreatureEvent(ENTRY_NOVOS, 1, novosEnterCombat)
RegisterCreatureEvent(ENTRY_NOVOS, 3, novosTargetDied)
RegisterCreatureEvent(ENTRY_NOVOS, 4, novosDied)
RegisterCreatureEvent(ENTRY_NOVOS, 23, novosReset)
RegisterCreatureEvent(ENTRY_NOVOS, 38, novosDoAction)

-- C++ spell_novos_summon_minions::HandleScript
-- (boss_novos.cpp:375-398): OnEffectHitTarget EFFECT_0
-- SPELL_EFFECT_SCRIPT_EFFECT -> the caster casts
-- SPELL_SUMMON_COPY_OF_MINIONS 59933 twice, triggered. The C++
-- target is nullptr (the DBC targeting resolves it); Lua passes
-- the caster explicitly, the kologarn stalker self-cast
-- precedent. The effectIndex gate is load-bearing: Go fires ev5
-- once per effect (razorscale summon_iron_dwarves precedent).
local function novosSummonMinions(event, spell, target, effectIndex)
    if effectIndex ~= 0 or target == nil then
        return
    end
    local caster = spell:GetCaster()
    if caster == nil then
        return
    end
    for _ = 1, 2 do
        caster:CastSpell(caster, SPELL_SUMMON_COPY_OF_MINIONS, true)
    end
end
RegisterSpellEvent(SPELL_SUMMON_MINIONS, 5, novosSummonMinions)
