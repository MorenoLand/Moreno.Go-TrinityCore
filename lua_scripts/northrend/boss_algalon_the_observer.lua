-- Algalon the Observer (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_algalon_the_observer.cpp
-- (1300 lines; 17 scripts — boss_algalon_the_observer (CreatureScript via
-- RegisterUlduarCreatureAI (BossAI), BOSS_ALGALON = 13);
-- npc_living_constellation / npc_black_hole / npc_collapsing_star /
-- npc_brann_bronzebeard_algalon (CreatureScript via
-- RegisterUlduarCreatureAI); go_celestial_planetarium_access
-- (GameObjectScript); ten spell_* scripts (spell_algalon_phase_punch /
-- spell_algalon_phase_constellation / spell_algalon_trigger_3_adds /
-- spell_algalon_collapse / spell_algalon_big_bang /
-- spell_algalon_remove_phase / spell_algalon_cosmic_smash /
-- spell_algalon_cosmic_smash_damage / spell_algalon_supermassive_fail /
-- spell_algalon_black_hole_phase_shifts (SpellScript / AuraScript));
-- achievement_he_feeds_on_your_tears (AchievementCriteriaScript); all
-- registered from inside AddSC_boss_algalon_the_observer(); loader decl
-- 123 / call 318 per northrend_script_loader.cpp — the FOURTEENTH group
-- of the "// Ulduar" block in AddNorthrendScripts(), immediately after
-- AddSC_boss_yogg_saron() (decl 122 / call 317); the call after it is
-- AddSC_instance_ulduar() — the Ulduar boss slice is closed by this run,
-- the Ulduar block's remaining unit is the instance script — loader order
-- confirmed this run; the checkpoint sequence (boss_yogg_saron ->
-- boss_algalon_the_observer) is followed).
-- Entry: 32871 Algalon (ulduar.h NPC_ALGALON, line 83 — entry-verifiable,
-- registration proceeds (the nexus_commanders / malygos / sartharion
-- kalecgos precedent); the CreatureScript ScriptName binding is DB-side
-- as usual). BOSS_ALGALON = 13 (ulduar.h line 49).
-- Sole-source verified: whole-server-tree grep for each of the seventeen
-- script names hits boss_algalon_the_observer.cpp (+ the loader decl/call
-- lines for AddSC_boss_algalon_the_observer) only; zero sql/ hits for all
-- seventeen. Lua body: ported Oct 2 09:46 in 8af8496; re-audited this run
-- — the event-3 slay (os.time() 1s rate-limit deadline), event-9 phase-two
-- threshold+latch, JustEngagedWith intro-Talk/_firstPull and the combat
-- timer arms are C++-exact.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3 OnTargetDied,
-- 4 OnDied, 9 OnDamageTaken, 23 OnReset.
-- Ported arms (C++-exact for all modeled arms):
-- JustEngagedWith intro Talks — the _firstPull flag (set true once in the
-- AI ctor, :291; never cleared by Reset()/Initialize(), :293-296) chooses
-- Talk(SAY_ALGALON_START_TIMER 3) + 26500ms intro (first pull, :377-387)
-- vs Talk(SAY_ALGALON_AGGRO 4) + 8s intro (later pulls, :370-376); mirrored
-- as a per-GUID Lua latch (script-lifetime, NOT wiped by event 23 —
-- C++-exact). First pull also schedules EVENT_START_COMBAT (16s, :554-559)
-- -> Talk(SAY_ALGALON_AGGRO 4) + PlayDirectMusic(ENGAGE_MUSIC_ID 15878)
-- (ported as creature:PlayMusic — the bridged SMSG_PLAY_MUSIC twin)
-- + instance->SetBossState(IN_PROGRESS) (PORTED tick 259); later pulls
-- write IN_PROGRESS in JustEngagedWith itself (BossAI passthrough,
-- PORTED tick 259).
-- The remaining JustEngagedWith legs (the SetFlag NOT_SELECTABLE /
-- SetImmuneToNPC set legs, summons.DespawnEntry(NPC_AZEROTH),
-- instance->SetData, BossAI::JustEngagedWith setActive leg) stay out: the set leg is bridgeable but the boss sits
-- REACT_PASSIVE from Reset through the intro, and C++ itself notes
-- (EVENT_INTRO_TIMER_DONE) that PASSIVE + NOT_SELECTABLE at combat
-- start makes the creature immediately evade — setting it at engage
-- would risk the same evade cancelling the intro timers before the
-- clear tick, and the compensating AttackStart leg IS ported in
-- the introdefensive tick below (AttackStart bridged,
-- lua_creature_events.go:6010; the threat-list leg is
-- #GetAITargets() + a GetVictim() nil-guard); only the
-- intro-timer clear is ported (see introdefensive). The
-- BossAI::JustEngagedWith DoZoneInCombat leg IS ported on later pulls
-- (algalonTimersStart); first-pull DoZoneInCombat was already ported.
-- InterruptNonMeleeSpells(:381) is called in code; the handler-surface
-- twin is a no-op (lua_creature_events.go:885, no in-flight cast state).
-- (CORRECTION 2026-10-23: the :384 first-pull DoZoneInCombat leg IS
-- ported below — creature:DoZoneInCombat() is bridged
-- (engine/world/lua_creature_events.go); the stale "no bridge" claim
-- is retracted for that leg.)
-- (CORRECTION 2026-10-22: the old "PlayDirectMusic has no bridge" claims
-- were stale — creature:PlayMusic IS bridged (lua_creature_events.go:118);
-- both engage music arms (:373 later pulls, :557 EVENT_START_COMBAT) are
-- ported below.)
-- KilledUnit — player-gated Talk(SAY_ALGALON_KILL 20) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious player-gated
-- variant precedent — at least the fortieth player-gated variant ported
-- (39 'player-gated' lua files git-born before it));
-- the C++ _hasYelled 1s rate limit (EVENT_UNLOCK_YELL) is modeled with
-- an os.time() per-guid deadline (the malchezaar os.time-deadline
-- precedent); 1s resolution caveat — the re-yell unlocks on the next
-- whole second rather than exactly 1000ms after the yell.
-- DamageTaken phase-two — Talk(SAY_ALGALON_PHASE_TWO 11) gated on
-- HealthBelowPctDamaged(20, damage) + the _phaseTwo latch (event 9; the
-- moroes threshold+latch precedent); the companion legs (summons
-- DespawnEntry NPC_LIVING_CONSTELLATION / NPC_COLLAPSING_STAR /
-- NPC_BLACK_HOLE / NPC_ALGALON_VOID_ZONE_VISUAL_STALKER, the stalker
-- KillAllEvents sweep, EVENT_SUMMON_COLLAPSING_STAR cancel, the four
-- worm-hole summons) have no bridges — documented only.
-- DOCUMENTED-ONLY (in this header; ENTRY_LIVING_CONSTELLATION 33052 is
-- registered too — entry-verifiable):
-- DamageTaken _fightWon branch — (health - damage) below 2.5% of max:
-- damage = 0 + the outro event machine (REACT_PASSIVE, AttackStop,
-- SPELL_SELF_STUN self-cast, summons.DespawnAll, EVENT_OUTRO_START ...)
-- — PORTED tick 259 (outroStart..outro11 below); the MovePoint arrival
-- gate for OUTRO_3 rides a documented 7s walk-time estimate, and the
-- ResetThreatList / summons.DespawnAll legs stay out (no bridges).
-- Timer Talks — 2026-10-14 retraction re-audit (C++ line refs): algalon's
-- Cast-state gates PORTED 2026-11-04: quantum strike / phase punch /
-- cosmic smash / ascend / big bang / collapsing star defer 500ms on
-- creature:IsCasting() (live, lua_creature_events.go:1950), no repeat
-- re-arm consumed — the mal_ganis idiom for the boss UpdateAI pre-loop
-- (:520) + in-loop (:702) CASTING gates, both PHASE_ROLE_PLAY-exempt.
-- The exemption is load-bearing: EVENT_INTRO_TIMER_DONE (the
-- introdefensive tick) and EVENT_START_COMBAT fire under PHASE_ROLE_PLAY
-- (engage sets it, INTRO_TIMER_DONE flips to NORMAL), so both stay
-- ungated per C++. The constellation arcane barrage stays ungated: its
-- own UpdateAI (:783) has no CASTING gate (it clears CASTING after cast).
-- own quantum strike / phase punch / cosmic smash / ascend arms ARE ported
-- on combat-start timers (event 1, C++-exact inits :392-397, header
-- corrected 2026-10-16); every other arm rides an unbridged machine
-- (stale "no-timer-bridge" claim RETRACTED — CreateLuaEvent is a real
-- bridge; the blocker is machine gating, gothik precedent). The
-- constellation's Arcane Barrage is also ported on an enter-combat timer
-- (event 1): 2.5s init + {4s,5s} repeat C++-exact (:732) but the Lua anchor
-- is EnterCombat where C++ anchors at Reset — anchor deviation noted.
-- COLLAPSING_STAR (18s+introDelay, :594-599) — Talk 5/6 + SummonCreature x4;
-- BIG_BANG (90s+introDelay) — Talk 7/8 + DoCastAOE(64443) PORTED
-- (bigBangTick, Repeat 90.5s); the SetPhase(PHASE_BIG_BANG) /
-- constellation DoAction(ACTION_BIG_BANG) / EVENT_RESUME_UPDATING
-- legs stay out. The no-target ACTION_ASCEND leg is PORTED tick 371
-- (Go-side count + DoAction 38 fire, algalonDoAction below).
-- Intro machine: Talk 0/1/2 (:529/:541/:546, EVENT_INTRO_1..3) chained under
-- PHASE_ROLE_PLAY from DoAction(ACTION_START_INTRO) (:328, Brann-summoned
-- pre-combat) — pre-combat roleplay, porting on combat-start timers would
-- fabricate them mid-fight. EVENT_INTRO_TIMER_DONE (:560-584) has NO Talk
-- arm (header correction this run) — pure machinery (SetPhase(NORMAL),
-- constellation summons) except the SetReactState(REACT_DEFENSIVE) (:566)
-- leg, which IS ported on the introDelay timer in algalonTimersStart,
-- along with the sheath / flag-clear legs, the triggered
-- SUPERMASSIVE_FAIL cast, and the 11 constellation summons;
-- Talk(AGGRO 4) fires in JustEngagedWith
-- non-first-pull (:372) and EVENT_START_COMBAT (:555, 16s, first-pull
-- only) — both now ported via the _firstPull latch above.
-- Outro machine: Talk 12..16 (:664-682, EVENT_OUTRO_6..10) chained from the
-- DamageTaken _fightWon branch (:483-495) via MovementInform spline arrival
-- (:413) and a Brann-summon DoAction(:657-661) — unbridged; a Talk-only
-- port would strand the boss without the outro.
-- Despawn machine: Talk 17..19 (:689-697, 5s/17s/26s) on
-- DoAction(EVENT_DESPAWN_ALGALON) — fired by the instance script on the
-- one-hour instance timer (instance_ulduar.cpp:774/1009, ulduar.h:499), not
-- combat — no bridge.
-- npc_brann_bronzebeard_algalon (34064 entry-verifiable, ulduar.h line
-- 230) — Talk(SAY_BRANN_ALGALON_INTRO_1 0) (EVENT_BRANN_SAY_INTRO_1),
-- Talk(SAY_BRANN_ALGALON_INTRO_2 1) (DoAction ACTION_INTRO_2),
-- Talk(SAY_BRANN_ALGALON_OUTRO 2) (EVENT_BRANN_OUTRO_1) — DoAction /
-- MovementInform / timer-gated; the senders are algalon's unported
-- intro/outro scheduler (:553/:660) and brann is unregistered
-- (dispatch IS bridged — ev38); no registration.
-- npc_collapsing_starAI DamageTaken — the _dying latch damage=0 rewrite +
-- SPELL_BLACK_HOLE_SPAWN_VISUAL / SPELL_SUMMON_BLACK_HOLE self-casts —
-- no Talk arm; the casts themselves are bridgeable (triggered
-- self-cast), but the summoned star AI has no registration yet
-- (the summon leg itself IS ported in collapsingStarTick — the stale
-- "no summon bridge" claim is retracted); the star arms stay out on
-- registration grounds (NPC_COLLAPSING_STAR 32955 entry-verifiable,
-- ulduar.h line 234).
-- npc_living_constellation / npc_black_hole — no Talk arms; no
-- registrations (NPC_LIVING_CONSTELLATION 33052 / NPC_BLACK_HOLE 32953
-- entry-verifiable, ulduar.h lines 232 / 235).
-- npc_living_constellation DoAction(ACTION_ACTIVATE_STAR) (:745-750):
-- SetReactState(REACT_AGGRESSIVE) + RemoveFlag(NOT_SELECTABLE) +
-- AttackStart + DoZoneInCombat — fired by the
-- spell_algalon_trigger_3_adds SpellScript (:1106-1112); dispatch IS
-- bridged (ev38) but the SpellScript sender is unported, so the
-- constellation's react flip stays out
-- (its Arcane Barrage enter-combat timer above is the only modeled arm).
-- boss DoAction(ACTION_INIT_ALGALON 6, ulduar.h:501 — instance
-- OnPlayerEnter timed-summon path, instance_ulduar.cpp:245, sets
-- _firstPull=false) — dispatch IS bridged (ev38) but the sender is
-- the unported instance script path; stays out, so a timed-summon
-- Algalon takes the first-pull intro in Lua where C++ takes the 8s one.
-- go_celestial_planetarium_access (GameObjectScript) — no GO bridge;
-- documented only.
-- The remaining spell scripts (spell_algalon_phase_constellation /
-- spell_algalon_trigger_3_adds / spell_algalon_big_bang /
-- spell_algalon_remove_phase / spell_algalon_cosmic_smash /
-- spell_algalon_cosmic_smash_damage / spell_algalon_supermassive_fail)
-- — SpellScript / AuraScript — no SpellScript / AuraScript bridges;
-- all join the no-SpellScript / no-AuraScript-bridge queues.
-- spell_algalon_phase_punch, spell_algalon_black_hole_phase_shifts
-- and spell_algalon_collapse are PORTED below (RegisterAuraEvent
-- periodic / apply / remove — the RemoveAura single-application twin
-- covers their RemoveAurasDueToSpell legs).
-- achievement_he_feeds_on_your_tears (OnCheck: GetData
-- DATA_HAS_FED_ON_TEARS) — PORTED 2026-11-08 (tick 416): the KilledUnit
-- latch rides algalonTargetDied below; Go case in
-- engine/world/achievement_criteria_scripts.go.

local ENTRY_ALGALON = 32871
local BOSS_ALGALON = 13 -- ulduar.h:49
local NPC_BRANN_BRONZEBEARD_ALG = 34064 -- ulduar.h:230

local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NOT_SELECTABLE = 0x02000000
local UNIT_FLAG_IMMUNE_TO_NPC = 0x200
local UNIT_FLAG_RENAME = 0x00000010
local FACTION_FRIENDLY = 35
local UNIT_STAND_STATE_STAND = 0
local UNIT_STAND_STATE_KNEEL = 8 -- UnitDefines.h:41

local SAY_ALGALON_START_TIMER = 3
local SAY_ALGALON_AGGRO = 4
local SAY_ALGALON_PHASE_TWO = 11
local SAY_ALGALON_OUTRO_1 = 12
local SAY_ALGALON_OUTRO_2 = 13
local SAY_ALGALON_OUTRO_3 = 14
local SAY_ALGALON_OUTRO_4 = 15
local SAY_ALGALON_OUTRO_5 = 16
local SAY_ALGALON_KILL = 20
local SPELL_SELF_STUN = 65256 -- boss_algalon_the_observer.cpp:82
local SPELL_KILL_CREDIT = 65184 -- boss_algalon_the_observer.cpp:83
local SPELL_TELEPORT = 62940 -- boss_algalon_the_observer.cpp:84
local SPELL_SUPERMASSIVE_FAIL = 65311 -- boss_algalon_the_observer.cpp:79 (moved above first use, tick 259)
local ACTION_OUTRO = 4 -- Actions enum, boss_algalon_the_observer.cpp:170-175
-- C++ ENGAGE_MUSIC_ID (boss_algalon_the_observer.cpp:64) — engage music,
-- played via PlayDirectMusic on every engage (:373 later pulls, :557
-- first-pull EVENT_START_COMBAT); creature:PlayMusic is the bridged twin
-- (SMSG_PLAY_MUSIC, lua_creature_events.go:118).
local ENGAGE_MUSIC_ID = 15878

-- Per-creature latch mirroring the AI members: _phaseTwo (one-shot at the
-- 20% threshold) and _hasYelled (the 1s kill-yell rate limit). C++
-- Reset() -> Initialize() clears both; event 23 does the same here.
local algalonState = {}

-- C++ _firstPull (boss_algalon_the_observer.cpp:291): set true once in the
-- AI ctor, never cleared by Reset()/Initialize() (:293-296) — mirrored as
-- a per-GUID latch that event 23 deliberately does NOT wipe (C++-exact);
-- event 4 (death -> fresh AI on respawn) clears it back to first-pull.
-- Absent key = true (never pulled since script load, like a fresh AI).
local algalonFirstPull = {}

local function getState(guid)
    local state = algalonState[guid]
    if state == nil then
        state = { phaseTwo = false, fightWon = false, killYellUntil = 0 }
        algalonState[guid] = state
    end
    return state
end

local timers = {}

-- C++ instance SetBossState(BOSS_ALGALON) item-level scan
-- (instance_ulduar.cpp:694-730): IN_PROGRESS scans every player in
-- the instance over EQUIPMENT_SLOT_START..END (0..18), skipping
-- BODY (3) and TABARD (18); weapon slots (MAINHAND 15..RANGED 17)
-- feed _maxWeaponItemLevel, the rest _maxArmorItemLevel. DONE
-- rechecks weapons only (15..17). The maxes never reset (only
-- Initialize clears them), so the Lua twin takes max(existing,
-- scanned) into the shared instanceData table, read by the Go
-- HERALD_OF_TITANS gate (checkUlduarCriteria). Slot constants are
-- Player.h:540-556. DORMANT without a characters DB: player
-- GetEquippedItemBySlot reads nil (lua_player.go) and the maxes
-- stay 0, which passes the gate exactly as the C++ ctor defaults
-- do before any scan.
local EQUIPMENT_SLOT_BODY = 3
local EQUIPMENT_SLOT_MAINHAND = 15
local EQUIPMENT_SLOT_RANGED = 17
local EQUIPMENT_SLOT_TABARD = 18
local EQUIPMENT_SLOT_END = 19

local function scanHeraldItemLevels(creature, weaponsOnly)
    local mapObj = creature:GetMap()
    if not mapObj then
        return
    end
    local data = mapObj:GetInstanceData()
    if not data then
        return
    end
    local players = mapObj:GetPlayers()
    if not players then
        return
    end
    local maxArmor = data.maxArmorItemLevel or 0
    local maxWeapon = data.maxWeaponItemLevel or 0
    local firstSlot, lastSlot = 0, EQUIPMENT_SLOT_END - 1
    if weaponsOnly then
        firstSlot, lastSlot = EQUIPMENT_SLOT_MAINHAND, EQUIPMENT_SLOT_RANGED
    end
    for _, player in ipairs(players) do
        for slot = firstSlot, lastSlot do
            if slot ~= EQUIPMENT_SLOT_BODY and slot ~= EQUIPMENT_SLOT_TABARD then
                local item = player:GetEquippedItemBySlot(slot)
                if item then
                    local level = item:GetItemLevel()
                    if level then
                        if slot >= EQUIPMENT_SLOT_MAINHAND and slot <= EQUIPMENT_SLOT_RANGED then
                            if level > maxWeapon then
                                maxWeapon = level
                            end
                        elseif level > maxArmor then
                            maxArmor = level
                        end
                    end
                end
            end
        end
    end
    data.maxArmorItemLevel = maxArmor
    data.maxWeaponItemLevel = maxWeapon
end

-- Forward declarations for the outro chain (tharon_ja/gal_darah
-- idiom: plain assignments below, never `local function` re-declares).
local outroStart, outro1, outro2, outro3, outro4, outro5, outro6, outro7, outro8, outro9, outro10, outro11

-- C++ DamageTaken: if (!_phaseTwo && me->HealthBelowPctDamaged(20,
-- damage)) { _phaseTwo = true; Talk(SAY_ALGALON_PHASE_TWO); <unbridgeable
-- despawn/summon legs> } — the moroes threshold+latch precedent.
-- The EVENT_SUMMON_COLLAPSING_STAR cancel (boss_algalon_the_
-- observer.cpp:473) is ported: the "collapsingstar" schedule key is
-- cancelled one-shot (the summons.DespawnEntry constellation/star/
-- black-hole/stalker legs + worm-hole DoSummon machine stay out —
-- documented only).
-- The _fightWon 2.5% branch (boss_algalon_the_observer.cpp:484-496):
-- damage = 0 + the outro event machine — PORTED tick 259 (the old
-- "deliberately not modeled" header note is retracted: every outro
-- leg now has a bridge except the MovePoint arrival gate, see
-- outro3 below). C++ order: _fightWon latch, damage = 0,
-- SetPhase(PHASE_ROLE_PLAY) (no phase bridge), REACT_PASSIVE,
-- AttackStop, NOT_SELECTABLE flag, DoCastSelf(SPELL_SELF_STUN),
-- events.Reset() (cancelTimers), summons.DespawnAll (no bridge),
-- EVENT_OUTRO_START 1.5s + EVENT_OUTRO_1 7s. No Talk arm in this
-- branch. The if/else-if is C++-exact: a hit crossing both the 20%
-- and 2.5% thresholds fires phase-two only.
local function algalonDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    local state = getState(guid)
    if state.fightWon then
        return false, 0
    end
    if state.phaseTwo then
        return
    end
    local maxHealth = creature:GetMaxHealth()
    if maxHealth == 0 then
        return
    end
    local postDamage = creature:GetHealth() - damage
    if postDamage < math.floor(maxHealth * 20 / 100) then
        state.phaseTwo = true
        creature:Talk(SAY_ALGALON_PHASE_TWO)
        local per = timers[guid]
        if per and per["collapsingstar"] then
            RemoveEventById(per["collapsingstar"])
            per["collapsingstar"] = nil
        end
    elseif postDamage < math.floor(maxHealth * 25 / 1000) then
        state.fightWon = true
        creature:SetReactState(0) -- REACT_PASSIVE
        creature:AttackStop()
        creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:CastSpell(creature, SPELL_SELF_STUN)
        -- events.Reset(): cancelTimers is defined below this function
        -- and out of scope here, so the timer-table clear is inlined
        -- (same body, same effect).
        local per = timers[guid]
        if per then
            for _, id in pairs(per) do
                RemoveEventById(id)
            end
            timers[guid] = nil
        end
        CreateLuaEvent(function()
            outroStart(creature, guid)
        end, 1500)
        CreateLuaEvent(function()
            outro1(creature, guid)
        end, 7000)
        return false, 0
    end
end

-- C++ KilledUnit: if (victim->GetTypeId() == TYPEID_PLAYER) {
-- _fedOnTears = true; if (!_hasYelled) { _hasYelled = true;
-- events.ScheduleEvent(EVENT_UNLOCK_YELL, 1s); Talk(SAY_ALGALON_KILL); } }
-- — the razuvious player-gated variant precedent; the 1s rate limit is
-- modeled with an os.time() deadline (the malchezaar os.time-deadline
-- precedent; re-yell unlocks on the next whole second).
local DATA_HAS_FED_ON_TEARS = 30043005 -- boss_algalon_the_observer.cpp:184

local function algalonTargetDied(event, creature, victim)
    if not victim or victim:GetObjectType() ~= "Player" then
        return
    end
    -- C++ KilledUnit :500-510: player victim -> _fedOnTears = true (the
    -- achievement_he_feeds_on_your_tears latch; ctor-only clear :290, so
    -- it deliberately survives ev23 Reset — the per-motion ScriptData
    -- store is fresh only on respawn, the C++ fresh-AI twin).
    creature:SetData(DATA_HAS_FED_ON_TEARS, 1)
    local state = getState(creature:GetGUID())
    local now = os.time()
    if now >= state.killYellUntil then
        state.killYellUntil = now + 1
        creature:Talk(SAY_ALGALON_KILL)
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

-- C++ outro event machine (boss_algalon_the_observer.cpp:630-690),
-- PORTED tick 259. All links ride raw one-shot CreateLuaEvents, not
-- the tracked timers table: C++ schedules them after events.Reset(),
-- and a later ev2/ev23/ev4 timer clear must not cancel the outro
-- (Algalon cannot evade mid-outro — self-stunned, REACT_PASSIVE,
-- threat cleared by AttackStop — so the C++ evade-cancel is
-- unreachable; the mimiron outtro-chain precedent).
-- EVENT_OUTRO_START (:635): SetBossState(BOSS_ALGALON 13, DONE).
outroStart = function(creature, guid)
    creature:SetBossState(BOSS_ALGALON, 3)
    scanHeraldItemLevels(creature, true) -- C++ DONE weapons recheck (instance_ulduar.cpp:700-709)
end

-- EVENT_OUTRO_1 (:637-642): RemoveAllAuras, ResetThreatList (no
-- bridge — stays out), SetFaction(FRIENDLY), RENAME flag, then
-- OUTRO_2 in 2s.
outro1 = function(creature, guid)
    creature:RemoveAllAuras()
    creature:SetFaction(FACTION_FRIENDLY)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_RENAME)
    CreateLuaEvent(function()
        outro2(creature, guid)
    end, 2000)
end

-- EVENT_OUTRO_2 (:643-648): SetRegenerateHealth(false), CombatStop,
-- MovePoint(POINT_ALGALON_OUTRO) — no MovePoint bridge, so the
-- MovementInform arrival gate (:405-414, OUTRO_3 1s after arrival)
-- is approximated: LandPos (1632.668, -302.7656) to OutroPos
-- (1633.64, -317.78) is ~15yd at walk 2.5yd/s (~6s) + 1s inform
-- delay = 7s. OUTRO_3 fires from this estimate, not from an
-- arrival — documented timing approximation.
outro2 = function(creature, guid)
    creature:SetRegenerateHealth(false)
    creature:CombatStop()
    CreateLuaEvent(function()
        outro3(creature, guid)
    end, 7000)
end

-- EVENT_OUTRO_3 (:649-653): DoCastAOE(SPELL_KILL_CREDIT)
-- (non-triggered self-cast), then OUTRO_4 in 1.4s, OUTRO_5 in 4.5s.
outro3 = function(creature, guid)
    creature:CastSpell(creature, SPELL_KILL_CREDIT)
    CreateLuaEvent(function()
        outro4(creature, guid)
    end, 1400)
    CreateLuaEvent(function()
        outro5(creature, guid)
    end, 4500)
end

-- EVENT_OUTRO_4 (:654-657): DoCastAOE(SPELL_SUPERMASSIVE_FAIL)
-- (non-triggered self-cast), RemoveFlag NOT_SELECTABLE.
outro4 = function(creature, guid)
    creature:CastSpell(creature, SPELL_SUPERMASSIVE_FAIL)
    creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
end

-- EVENT_OUTRO_5 (:658-662): SummonCreature Brann at BrannOutroPos
-- (boss_algalon_the_observer.cpp:233) + Brann DoAction(ACTION_OUTRO)
-- (the Brann outro AI is unported, so the dispatch is a no-op until
-- it lands — sender side is C++-exact); then OUTRO_6 in 10s.
outro5 = function(creature, guid)
    local brann = creature:SummonCreature(NPC_BRANN_BRONZEBEARD_ALG, 1631.989, -221.5439, 418.4091, 4.714909, 7, 0)
    if brann then
        brann:DoAction(ACTION_OUTRO)
    end
    CreateLuaEvent(function()
        outro6(creature, guid)
    end, 10000)
end

-- EVENT_OUTRO_6 (:663-667): Talk(SAY_ALGALON_OUTRO_1),
-- SetStandState(KNEEL), then OUTRO_7 in 39s.
outro6 = function(creature, guid)
    creature:Talk(SAY_ALGALON_OUTRO_1)
    creature:SetStandState(UNIT_STAND_STATE_KNEEL)
    CreateLuaEvent(function()
        outro7(creature, guid)
    end, 39000)
end

-- EVENT_OUTRO_7 (:668-671): Talk(SAY_ALGALON_OUTRO_2), then OUTRO_8 in 18s.
outro7 = function(creature, guid)
    creature:Talk(SAY_ALGALON_OUTRO_2)
    CreateLuaEvent(function()
        outro8(creature, guid)
    end, 18000)
end

-- EVENT_OUTRO_8 (:672-675): Talk(SAY_ALGALON_OUTRO_3), then OUTRO_9 in 12s.
outro8 = function(creature, guid)
    creature:Talk(SAY_ALGALON_OUTRO_3)
    CreateLuaEvent(function()
        outro9(creature, guid)
    end, 12000)
end

-- EVENT_OUTRO_9 (:676-679): Talk(SAY_ALGALON_OUTRO_4), then OUTRO_10 in 24s.
outro9 = function(creature, guid)
    creature:Talk(SAY_ALGALON_OUTRO_4)
    CreateLuaEvent(function()
        outro10(creature, guid)
    end, 24000)
end

-- EVENT_OUTRO_10 (:680-683): Talk(SAY_ALGALON_OUTRO_5), then OUTRO_11 in 15s.
outro10 = function(creature, guid)
    creature:Talk(SAY_ALGALON_OUTRO_5)
    CreateLuaEvent(function()
        outro11(creature, guid)
    end, 15000)
end

-- EVENT_OUTRO_11 (:684-688): SetStandState(STAND), DoCastSelf(
-- SPELL_TELEPORT), DespawnOrUnsummon(1.2s).
outro11 = function(creature, guid)
    creature:SetStandState(UNIT_STAND_STATE_STAND)
    creature:CastSpell(creature, SPELL_TELEPORT)
    creature:DespawnOrUnsummon(1200)
end

local function algalonReset(event, creature)
    algalonState[creature:GetGUID()] = nil
    cancelTimers(creature:GetGUID())
    -- EnterEvadeMode (boss_algalon_the_observer.cpp:449-455):
    -- SetBossState(FAIL) then BossAI::EnterEvadeMode -> Reset() ->
    -- _Reset() NOT_STARTED — PORTED tick 259 (FAIL first, then
    -- NOT_STARTED, C++ order; the FAIL write is transient in the
    -- same handler, matching C++ where Reset overwrites it at once).
    -- Alive-gated: ev23 also pre-fires on the Go death path, where
    -- C++ EnterEvadeMode never runs.
    if creature:IsAlive() then
        creature:SetBossState(BOSS_ALGALON, 2) -- FAIL
        creature:SetBossState(BOSS_ALGALON, 0) -- _Reset() NOT_STARTED
    end
    -- The SetSheath(UNARMED) leg is ported — Algalon drew his weapon
    -- at the intro timer (:563 MELEE).
    creature:SetSheath(0) -- SHEATH_STATE_UNARMED
    -- C++ boss_algalon_the_observer.cpp:305: me->SetReactState(REACT_PASSIVE)
    -- (Reset arm order: _Reset -> flip -> Initialize).
    creature:SetReactState(0) -- REACT_PASSIVE
end

RegisterCreatureEvent(ENTRY_ALGALON, 3, algalonTargetDied)
RegisterCreatureEvent(ENTRY_ALGALON, 9, algalonDamageTaken)
RegisterCreatureEvent(ENTRY_ALGALON, 23, algalonReset)

-- C++ spell_algalon_supermassive_fail::RecalculateDamage
-- (boss_algalon_the_observer.cpp:1229-1236, OnHit, spell 65311): a
-- hit player resets the NO_SPELL_HIT achievement criteria for this
-- spell (evenIfCriteriaComplete = true). The arm mutates no damage,
-- so timing is irrelevant and the caster-side SpellHitTarget leg
-- (event 15) is the exact carrier — it fires once per landed hit
-- target and skips evaded/immune/missed targets, the same observable
-- outcome as C++ GetHitPlayer() being nil on a miss. PORTED tick
-- 367 via the new player:ResetAchievementCriteria bridge
-- (condition 9 = ACHIEVEMENT_CRITERIA_CONDITION_NO_SPELL_HIT; the
-- bridge documents the evenIfCriteriaComplete delta: Go skips
-- already-completed criteria, C++ with true resets them).
local function algalonSupermassiveFail(event, creature, target, spellId)
    if spellId == SPELL_SUPERMASSIVE_FAIL and target:GetObjectType() == "Player" then
        target:ResetAchievementCriteria(9, SPELL_SUPERMASSIVE_FAIL)
    end
end

RegisterCreatureEvent(ENTRY_ALGALON, 15, algalonSupermassiveFail)

-- Timer-driven cast arms (the maiden CreateLuaEvent convention —
-- GUID-keyed timers table, cancel on events 2/4/23;
-- CastSpell(nil, spell) = DoCastVictim victim-cast,
-- CastSpell(creature, spell) = DoCastAOE/DoCastSelf self-cast). All four
-- boss arms are scheduled in JustEngagedWith with NO phase/group args —
-- valid in all phases (phase-arg arms like the voice-of-Yogg-Saron kit are
-- the gothik-blocked kind; these are not). DOCUMENTED DEVIATIONS: (1)
-- introDelay is selected by the _firstPull latch (26500ms first pull /
-- 8s later pulls, :364-386) — the 2026-10-16 "re-pull 8s path always"
-- deviation is superseded by the latch port above. (2)
-- EVENT_ASCEND_TO_THE_HEAVENS schedules EVENT_EVADE 2.5s later
-- (EnterEvadeMode); only the Talk+cast arm is ported, so the lua
-- boss stays in combat ~2.5s past enrage — the EVADE trigger itself
-- has no bridge (no EnterEvadeMode binding, event 25 unused). The
-- evade SetBossState FAIL + _Reset NOT_STARTED legs ARE ported in
-- algalonReset (tick 259) for any evade the Go engine does drive.
-- The evade SetSheath(SHEATH_STATE_UNARMED) leg IS ported (algalonReset, bridge
-- landed 2026-10-20). (3)
-- (3) EVENT_SUMMON_COLLAPSING_STAR (Talk 5/6 + four SummonCreature legs)
-- is PORTED (2026-10-23, creature:SummonCreature bridge — 'no summon
-- bridge' retracted) — collapsingstar key cancel on phase-2. (4)
-- EVENT_BIG_BANG stays out:
-- the arm sets PHASE_BIG_BANG, orders constellation DoActions via a
-- grid search and reschedules the phase restore — phase-machine legs block
-- it (gothik precedent). (5) npc_living_constellation's
-- EVENT_ARCANE_BARRAGE carries SPELLVALUE_MAX_TARGETS=1 which the
-- three-arg CastSpell bridge cannot express (documented, same class as
-- brightleaf's flux-stacks deviation).
local SPELL_QUANTUM_STRIKE = 64395
local SPELL_PHASE_PUNCH = 64412
local SPELL_ASCEND_TO_THE_HEAVENS = 64487
local SPELL_COSMIC_SMASH = 62301
local SPELL_ARCANE_BARRAGE = 64599
local SPELL_COLLAPSE = 62018 -- boss_algalon_the_observer.cpp:94
local SPELL_BIG_BANG = 64443 -- boss_algalon_the_observer.cpp:82
local SPELL_BLACK_HOLE_PHASE_SHIFTS = 62168
local SPELL_BLACK_HOLE_DOT = 62169 -- boss_algalon_the_observer.cpp:106

local ENTRY_LIVING_CONSTELLATION = 33052

local SAY_ALGALON_BIG_BANG = 7
local EMOTE_ALGALON_BIG_BANG = 8
local SAY_ALGALON_ASCEND = 9
local EMOTE_ALGALON_COSMIC_SMASH = 10
local SAY_ALGALON_COLLAPSING_STAR = 5
local EMOTE_ALGALON_COLLAPSING_STAR = 6

-- boss_algalon_the_observer.cpp:221-226 (NPC_COLLAPSING_STAR 32955,
-- ulduar.h line 234).
local NPC_COLLAPSING_STAR = 32955
local COLLAPSING_STAR_POSITIONS = {
    {1649.438, -319.8127, 418.3941, 1.082104},
    {1647.005, -288.6790, 417.3955, 3.490659},
    {1622.451, -321.1563, 417.6188, 4.677482},
    {1615.060, -291.6816, 417.7796, 3.490659},
}

-- Later-pull introDelay (:376); the first-pull 26500ms path is selected
-- by the _firstPull latch in algalonTimersStart above.
local INTRO_DELAY = 8000

-- C++ boss_algalon_the_observer.cpp:221-231 (LIVING_CONSTELLATION_COUNT
-- 11, ConstellationPos literals).
local CONSTELLATION_POSITIONS = {
    {1625.208, -267.2771, 446.4296, 5.044002},
    {1658.279, -262.5490, 441.9073, 4.188790},
    {1678.677, -276.3280, 427.7531, 3.979351},
    {1593.389, -299.4325, 432.4636, 6.073746},
    {1685.613, -300.1219, 443.2366, 3.385939},
    {1591.706, -263.8201, 441.4153, 5.253441},
    {1668.317, -324.7676, 457.9394, 3.211406},
    {1592.242, -325.5323, 446.9508, 0.226893},
    {1635.821, -363.3442, 424.3459, 1.466077},
    {1672.188, -357.2484, 436.7337, 2.338741},
    {1615.800, -348.0065, 442.9586, 1.134464},
}

-- C++ boss_algalon_the_observerAI::EVENT_QUANTUM_STRIKE:
-- DoCastVictim(SPELL_QUANTUM_STRIKE 64395) (non-triggered),
-- 3500ms+introDelay init, Repeat(3s, 5s) (:392, :586-589 verified).
local function quantumStrikeTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "quantumstrike", 500, function()
            quantumStrikeTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_QUANTUM_STRIKE)
    schedule(guid, "quantumstrike", {3000, 5000}, function()
        quantumStrikeTick(creature, guid)
    end)
end

-- C++ EVENT_PHASE_PUNCH: DoCastVictim(SPELL_PHASE_PUNCH 64412)
-- (non-triggered), 15500ms+introDelay init, Repeat(15500ms)
-- (:393, :590-593 verified).
local function phasePunchTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "phasepunch", 500, function()
            phasePunchTick(creature, guid)
        end)
        return
    end
    creature:CastSpell(nil, SPELL_PHASE_PUNCH)
    schedule(guid, "phasepunch", 15500, function()
        phasePunchTick(creature, guid)
    end)
end

-- C++ EVENT_COSMIC_SMASH: Talk(EMOTE_ALGALON_COSMIC_SMASH 10) +
-- DoCastAOE(SPELL_COSMIC_SMASH 62301) (non-triggered), 25s+introDelay
-- init, Repeat(25500ms) (:397, :626-630 verified).
local function cosmicSmashTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "cosmicsmash", 500, function()
            cosmicSmashTick(creature, guid)
        end)
        return
    end
    creature:Talk(EMOTE_ALGALON_COSMIC_SMASH)
    creature:CastSpell(creature, SPELL_COSMIC_SMASH)
    schedule(guid, "cosmicsmash", 25500, function()
        cosmicSmashTick(creature, guid)
    end)
end

-- C++ EVENT_ASCEND_TO_THE_HEAVENS: Talk(SAY_ALGALON_ASCEND 9) +
-- DoCastAOE(SPELL_ASCEND_TO_THE_HEAVENS 64487) (non-triggered),
-- 360s+introDelay init, one-shot (:396, :618-622 verified).
local function ascendTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "ascend", 500, function()
            ascendTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_ALGALON_ASCEND)
    creature:CastSpell(creature, SPELL_ASCEND_TO_THE_HEAVENS)
end

-- C++ EVENT_BIG_BANG: Talk(SAY_ALGALON_BIG_BANG 7) +
-- Talk(EMOTE_ALGALON_BIG_BANG 8) + DoCastAOE(SPELL_BIG_BANG 64443)
-- (non-triggered), 90s+introDelay init, Repeat(1min+30s+500ms).
-- The phase-machine legs (SetPhase PHASE_BIG_BANG, constellation
-- DoAction ACTION_BIG_BANG, EVENT_RESUME_UPDATING 9s+500ms) stay out.
-- The no-target ACTION_ASCEND leg (spell_algalon_big_bang: if zero
-- EFFECT_0 area-enemy targets, DoAction ACTION_ASCEND — cancels
-- EVENT_RESUME_UPDATING and schedules EVENT_ASCEND_TO_THE_HEAVENS
-- 1.5s) is PORTED tick 371: the count + AfterCast fire live Go-side
-- (creatureAlgalonBigBangAscend, boss_ai.go) and reach algalonDoAction
-- below as CREATURE_EVENT_ON_DO_ACTION (38) with action 3; the Lua
-- model has no resume timer, so the only observable leg is the
-- 1.5s ascend reschedule.
local function bigBangTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "bigbang", 500, function()
            bigBangTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_ALGALON_BIG_BANG)
    creature:Talk(EMOTE_ALGALON_BIG_BANG)
    creature:CastSpell(creature, SPELL_BIG_BANG)
    schedule(guid, "bigbang", 90500, function()
        bigBangTick(creature, guid)
    end)
end

-- C++ spell_algalon_big_bang::CheckTargets + boss_algalon_the_observerAI::
-- DoAction(ACTION_ASCEND) (boss_algalon_the_observer.cpp:331-335, :1139-
-- 1164): when Big Bang (64443) hits zero EFFECT_0 area-enemy targets,
-- Algalon cancels EVENT_RESUME_UPDATING and schedules
-- EVENT_ASCEND_TO_THE_HEAVENS in 1.5s. The count + AfterCast fire live
-- Go-side (creatureAlgalonBigBangAscend, boss_ai.go) and reach here as
-- CREATURE_EVENT_ON_DO_ACTION (38) with action 3. The Lua model has no
-- phase machine and no resume timer, so the only observable leg is the
-- reschedule: schedule() replaces the 360s "ascend" timer like C++
-- EventMap (keyed by event id). PORTED tick 371.
local ACTION_ASCEND = 3 -- boss_algalon_the_observer.cpp:169

local function algalonDoAction(event, creature, action)
    if action == ACTION_ASCEND then
        schedule(creature:GetGUID(), "ascend", 1500, function()
            ascendTick(creature, creature:GetGUID())
        end)
    end
end

-- C++ EVENT_SUMMON_COLLAPSING_STAR: Talk(SAY_ALGALON_COLLAPSING_
-- STAR 5) + Talk(EMOTE_ALGALON_COLLAPSING_STAR 6) + 4x SummonCreature(
-- NPC_COLLAPSING_STAR 32955, CollapsingStarPos literals,
-- TEMPSUMMON_CORPSE_DESPAWN Eluna type 4), 18s+introDelay init,
-- Repeat(1min) (:394, :594-600 verified). The stale "no summon
-- bridge" claim is retracted for this leg. Delta: the star AI's own arms
-- are unported, but the boss JustSummoned star case is ported below
-- (algalonJustSummoned: PASSIVE + triggered COLLAPSE; MoveRandom out).
-- C++ boss_algalon_the_observerAI::JustSummoned (boss_algalon_the_observer.cpp:417-425),
-- collapsing-star case only: SetReactState PASSIVE + triggered COLLAPSE
-- self-cast, in C++ order. MoveRandom(20.0) stays out (no Move* bridge,
-- moverandom census). The other switch cases stay out: void-zone /
-- asteroid / worm-hole stalkers and dark matter are never summoned by
-- the Lua (only COLLAPSING_STAR 32955 is), so their arms would be dead
-- code; summons.Summon bookkeeping has no Go twin.
local function algalonJustSummoned(event, creature, summon)
    if summon:GetEntry() ~= NPC_COLLAPSING_STAR then
        return
    end
    summon:SetReactState(0) -- REACT_PASSIVE
    summon:CastSpell(summon, SPELL_COLLAPSE, true)
end

RegisterCreatureEvent(ENTRY_ALGALON, 19, algalonJustSummoned)

local function collapsingStarTick(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "collapsingstar", 500, function()
            collapsingStarTick(creature, guid)
        end)
        return
    end
    creature:Talk(SAY_ALGALON_COLLAPSING_STAR)
    creature:Talk(EMOTE_ALGALON_COLLAPSING_STAR)
    for _, pos in ipairs(COLLAPSING_STAR_POSITIONS) do
        creature:SummonCreature(NPC_COLLAPSING_STAR, pos[1], pos[2], pos[3], pos[4], 4, 0)
    end
    schedule(guid, "collapsingstar", 60000, function()
        collapsingStarTick(creature, guid)
    end)
end

-- C++ JustEngagedWith (boss_algalon_the_observer.cpp:362-386): the
-- _firstPull latch chooses the arm. First pull (:377-387):
-- Talk(SAY_ALGALON_START_TIMER 3), introDelay = 26500ms,
-- EVENT_START_COMBAT scheduled in 16s. Later pulls (:370-376):
-- Talk(SAY_ALGALON_AGGRO 4), introDelay = 8s. The combat timer arms
-- below ride the same introDelay (the maiden CreateLuaEvent convention).
local function algalonTimersStart(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    local introDelay
    if algalonFirstPull[guid] == nil then
        algalonFirstPull[guid] = false
        creature:Talk(SAY_ALGALON_START_TIMER)
        creature:InterruptNonMeleeSpells(false) -- C++ :381, after Talk, before setActive/SetFarVisible.
        -- C++ JustEngagedWith first-pull (boss_algalon_the_observer.cpp:383-384):
        -- me->SetFarVisible(true), then DoZoneInCombat() (C++ :383-384).
        -- InterruptNonMeleeSpells(false) ported above; setActive leg has no bridge.
        creature:SetFarVisible(true)
        creature:DoZoneInCombat() -- C++ :384, in arm order after SetFarVisible (bridge live 2026-10-23)
        introDelay = 26500
        -- C++ EVENT_START_COMBAT (:554-559, first-pull only, 16s):
        -- Talk(SAY_ALGALON_AGGRO 4) + PlayDirectMusic(ENGAGE_MUSIC_ID)
        -- + instance->SetBossState(IN_PROGRESS) (PORTED tick 259).
        -- cancelTimers above mirrors the C++ events.Reset() (:366),
        -- which also wipes a pending START_COMBAT on re-pull.
        schedule(guid, "startcombat", 16000, function()
            creature:Talk(SAY_ALGALON_AGGRO)
            creature:PlayMusic(ENGAGE_MUSIC_ID)
            creature:SetBossState(BOSS_ALGALON, 1)
            scanHeraldItemLevels(creature, false) -- C++ IN_PROGRESS full scan (instance_ulduar.cpp:711-730)
        end)
    else
        -- C++ JustEngagedWith later-pull leg (:370-376):
        -- Talk(SAY_ALGALON_AGGRO 4) + PlayDirectMusic(ENGAGE_MUSIC_ID)
        -- + BossAI::JustEngagedWith (SetBossState IN_PROGRESS PORTED
        -- tick 259, base-first inside the passthrough; DoZoneInCombat
        -- leg; setActive leg has no bridge).
        creature:Talk(SAY_ALGALON_AGGRO)
        creature:PlayMusic(ENGAGE_MUSIC_ID)
        creature:SetBossState(BOSS_ALGALON, 1)
        scanHeraldItemLevels(creature, false) -- C++ IN_PROGRESS full scan (instance_ulduar.cpp:711-730)
        creature:SetCombatPulseDelay(5) -- BossAI::JustEngagedWith arm 1, later-pull branch only (boss_algalon_the_observer.cpp:374; ScriptedCreature.cpp:540)
        creature:DoZoneInCombat()
        introDelay = INTRO_DELAY
    end
    -- C++ EVENT_INTRO_TIMER_DONE (:560-584): SetPhase(PHASE_NORMAL),
    -- SetSheath(SHEATH_STATE_MELEE), RemoveFlag(NOT_SELECTABLE),
    -- SetImmuneToNPC(false), SetReactState(REACT_DEFENSIVE) (:566),
    -- DoCastAOE(SUPERMASSIVE_FAIL) + the AttackStart workaround +
    -- the REACT_PASSIVE constellation summons. The flag clear rides
    -- this tick in C++ order (before the react flip); it is safe
    -- without the unported JustEngagedWith set leg (a clear can never
    -- strand combat, unlike a set). SetImmuneToNPC(false) is ported
    -- in the introdefensive tick below (C++ order); the triggered
    -- DoCastAOE(SUPERMASSIVE_FAIL) and the 11 REACT_PASSIVE
    -- constellation summons are ported in the same tick, as is
    -- the AttackStart workaround (:571-572, see the tick body);
    -- only the random-stalker ActivateLivingConstellation event
    -- stays out. Without
    -- the react flip the boss keeps the algalonReset REACT_PASSIVE
    -- while engaged and the engine's isCreaturePassive gate never
    -- lets it attack.
    schedule(guid, "introdefensive", introDelay, function()
        creature:SetSheath(1) -- SHEATH_STATE_MELEE (EVENT_INTRO_TIMER_DONE :563, C++ order before the flag clear)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_NOT_SELECTABLE)
        creature:RemoveFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_IMMUNE_TO_NPC) -- EVENT_INTRO_TIMER_DONE :565, C++ order after the NOT_SELECTABLE clear
        creature:SetReactState(1) -- REACT_DEFENSIVE
        creature:CastSpell(creature, SPELL_SUPERMASSIVE_FAIL, true) -- DoCastAOE triggered, C++ order after the react flip
        for _, pos in ipairs(CONSTELLATION_POSITIONS) do
            local summon = creature:SummonCreature(ENTRY_LIVING_CONSTELLATION, pos[1], pos[2], pos[3], pos[4], 6, 0)
            if summon then
                summon:SetReactState(0) -- REACT_PASSIVE, as summoned by C++
            end
        end
        -- C++ EVENT_INTRO_TIMER_DONE (:571-572): the AttackStart
        -- workaround for creatures starting combat REACT_PASSIVE +
        -- NOT_SELECTABLE — if (!IsThreatListEmpty())
        -- AttackStart(GetCurrentVictim()); the threat-list leg is
        -- #GetAITargets() and GetVictim() reads TargetGUID
        -- (nil-guard; it may be 0). AttackStart is bridged
        -- (lua_creature_events.go:6010).
        if #creature:GetAITargets() > 0 then
            local v = creature:GetVictim()
            if v then
                creature:AttackStart(v)
            end
        end
    end)
    schedule(guid, "quantumstrike", 3500 + introDelay, function()
        quantumStrikeTick(creature, guid)
    end)
    schedule(guid, "phasepunch", 15500 + introDelay, function()
        phasePunchTick(creature, guid)
    end)
    schedule(guid, "cosmicsmash", 25000 + introDelay, function()
        cosmicSmashTick(creature, guid)
    end)
    schedule(guid, "bigbang", 90000 + introDelay, function()
        bigBangTick(creature, guid)
    end)
    schedule(guid, "ascend", 360000 + introDelay, function()
        ascendTick(creature, guid)
    end)
    schedule(guid, "collapsingstar", 18000 + introDelay, function()
        collapsingStarTick(creature, guid)
    end)
end

local function algalonTimersStop(event, creature)
    cancelTimers(creature:GetGUID())
    -- C++ _firstPull lives on the AI object: death -> despawn -> respawn
    -- constructs a fresh AI with _firstPull = true (boss_algalon_the_
    -- observer.cpp:291). Wipes (event 2) keep the latch; only death
    -- re-arms the first-pull intro. Gated on entry — the shared handler
    -- also serves the living constellation.
    if event == 4 and creature:GetEntry() == ENTRY_ALGALON then
        algalonFirstPull[creature:GetGUID()] = nil
    end
end

-- C++ npc_living_constellationAI::EVENT_ARCANE_BARRAGE:
-- DoCastAOE(SPELL_ARCANE_BARRAGE 64599) with SPELLVALUE_MAX_TARGETS=1
-- (non-triggered; the extra-args form, not the triggered bool), 2.5s init,
-- Repeat(4s, 5s) — scheduled in Reset with no phase arg (:732, :794-800
-- verified). Lua anchors the first schedule at EnterCombat (event 1),
-- C++ at Reset — value C++-exact, anchor deviation documented. The
-- max-targets leg is unbridgeable — documented above.
local function arcaneBarrageTick(creature, guid)
    creature:CastSpell(creature, SPELL_ARCANE_BARRAGE)
    schedule(guid, "arcanebarrage", {4000, 5000}, function()
        arcaneBarrageTick(creature, guid)
    end)
end

local function constellationEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "arcanebarrage", 2500, function()
        arcaneBarrageTick(creature, guid)
    end)
end

RegisterCreatureEvent(ENTRY_ALGALON, 1, algalonTimersStart)
RegisterCreatureEvent(ENTRY_ALGALON, 2, algalonTimersStop)
RegisterCreatureEvent(ENTRY_ALGALON, 4, algalonTimersStop)
RegisterCreatureEvent(ENTRY_ALGALON, 38, algalonDoAction)
RegisterCreatureEvent(ENTRY_LIVING_CONSTELLATION, 1, constellationEnterCombat)
RegisterCreatureEvent(ENTRY_LIVING_CONSTELLATION, 2, algalonTimersStop)
RegisterCreatureEvent(ENTRY_LIVING_CONSTELLATION, 4, algalonTimersStop)
RegisterCreatureEvent(ENTRY_LIVING_CONSTELLATION, 23, algalonTimersStop)

-- C++ spell_algalon_black_hole_phase_shifts::HandleEffectApply
-- (boss_algalon_the_observer.cpp:1258-1262): EFFECT_0 PHASE. The
-- Load() gate (:1249-1252) admits player owners only; the owner
-- casts SPELL_BLACK_HOLE_DOT (62169) triggered on itself. The
-- engine fires this hook from the aura-apply path (RegisterAuraEvent,
-- AuraEventOnApply = 2; tick-196 hook). No PreventDefaultAction.
local function blackHolePhaseShiftsApply(event, aura, target)
    if target:GetObjectType() ~= "Player" then
        return
    end
    target:CastSpell(target, SPELL_BLACK_HOLE_DOT, true)
end

RegisterAuraEvent(SPELL_BLACK_HOLE_PHASE_SHIFTS, 2, blackHolePhaseShiftsApply)

-- C++ spell_algalon_phase_punch (boss_algalon_the_observer.cpp:1040-1062):
-- EFFECT_0 SPELL_AURA_PERIODIC_DUMMY. Each tick PreventDefaultAction;
-- when the stack is above 1 the previous stack's alpha aura drops, then
-- the target casts this stack's alpha aura on itself, triggered; at 5
-- stacks the aura removes itself. OnRemove drops this stack's alpha
-- aura unless the stack is 5. PhasePunchAlphaId (cpp:113) is
-- {64435, 64434, 64428, 64421, 64417} (0-indexed in C++).
local PHASE_PUNCH_ALPHA_IDS = {64435, 64434, 64428, 64421, 64417}

local function phasePunchPeriodic(event, aura, target)
    local stacks = aura:GetStackAmount()
    if stacks ~= 1 then
        target:RemoveAura(PHASE_PUNCH_ALPHA_IDS[stacks - 1])
    end
    target:CastSpell(target, PHASE_PUNCH_ALPHA_IDS[stacks], true)
    if stacks == 5 then
        aura:Remove()
    end
    return true
end

local function phasePunchRemove(event, aura, target)
    local stacks = aura:GetStackAmount()
    if stacks ~= 5 then
        target:RemoveAura(PHASE_PUNCH_ALPHA_IDS[stacks])
    end
end

RegisterAuraEvent(SPELL_PHASE_PUNCH, 1, phasePunchPeriodic)
RegisterAuraEvent(SPELL_PHASE_PUNCH, 3, phasePunchRemove)

-- C++ spell_algalon_collapse (boss_algalon_the_observer.cpp:1121-1137):
-- EFFECT_0 SPELL_AURA_PERIODIC_DUMMY. Each tick PreventDefaultAction;
-- the target deals 1% of its max health to itself (NODAMAGE).
-- DealDamage's default school-7 leg is the NODAMAGE twin (raw damage,
-- no armor/absorb — the armor leg lives in the spell-ID branch of
-- luaUnitDealDamage); CountPctFromMaxHealth is the live builder-owned
-- read on the creature surface. The aura is cast triggered on the
-- collapsing-star summons (algalonJustSummoned, above).
local function collapseTick(event, aura, target)
    target:DealDamage(target, target:CountPctFromMaxHealth(1))
    return true
end

RegisterAuraEvent(SPELL_COLLAPSE, 1, collapseTick)
