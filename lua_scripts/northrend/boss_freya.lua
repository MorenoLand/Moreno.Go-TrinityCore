-- Freya (Ulduar) — Lua port of
-- src/server/scripts/Northrend/Ulduar/Ulduar/boss_freya.cpp
-- (boss_freya (CreatureScript) via GetUlduarAI<boss_freyaAI>
-- (BossAI), BOSS_FREYA = 9; boss_elder_brightleaf /
-- boss_elder_ironbranch / boss_elder_stonebark (CreatureScript
-- via GetUlduarAI<...AI> (BossAI), BOSS_BRIGHTLEAF = 14 /
-- BOSS_IRONBRANCH = 15 / BOSS_STONEBARK = 16 — all registered
-- from inside AddSC_boss_freya(); loader decl 120 / call 315
-- per northrend_script_loader.cpp — the ELEVENTH group of the
-- "// Ulduar" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_hodir() (decl 119 / call 314); the call
-- after it is AddSC_boss_thorim()).
-- Entry: 32906 Freya (ulduar.h NPC_FREYA, line 80),
-- 32913 Ironbranch (NPC_IRONBRANCH, line 136),
-- 32915 Brightleaf (NPC_BRIGHTLEAF, line 137),
-- 32914 Stonebark (NPC_STONEBARK, line 138) — all
-- entry-verifiable; the CreatureScript ScriptName bindings are
-- DB-side as usual.
-- TIMER-BRIDGE DOCTRINE (2026-10-13, supersedes the stale
-- "no timer-event / cast bridges" claims this header used to
-- carry): CreateLuaEvent is a real registered bridge
-- (engine/scripting/runtime.go); timer-driven cast/Talk arms
-- on the main AI port via the GUID-keyed timers table below
-- (cancel on events 2/4/23), following the maiden_of_virtue /
-- moroes in-tree precedent. CastSpell(nil, spell) =
-- DoCastVictim; CastSpell(creature, spell) = DoCastSelf /
-- DoCastAOE; the 3rd arg carries the C++ DoCast triggered
-- flag (2-arg overload = non-triggered). Phase-machine-gated,
-- summon/instance-GUID, adds/hard-mode, MotionMaster and
-- DoAction-gated arms stay out; random-target legs bridge via
-- the baltharus/maiden GetPlayersInWorld candidate-selection
-- precedent (thorim colossus RUNIC_CHARGE / giant
-- RUNE_DETONATION this run).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat,
-- 3 OnTargetDied, 4 OnDied, 9 OnDamageTaken, 23 OnReset.
--
-- PORTED ARMS (all C++-exact unless noted under DEVIATIONS):
-- Freya JustEngagedWith (:388-391) unconditional timers:
--   EVENT_ENRAGE (:421-424, init 10min :390, one-shot — no
--     re-arm in C++) — Talk(SAY_BERSERK 4) + DoCast(me,
--     SPELL_ENRAGE 47008), non-triggered self-cast.
--   EVENT_EONAR_GIFT (:446-449, init 25s :389, repeat
--     40s-50s) — Talk(EMOTE_LIFEBINDERS_GIFT 8) + DoCast(me,
--     SPELL_SUMMON_EONAR_GIFT 62572), non-triggered self-cast
--     (the NPC 33228 summon is spell-effect-side; the AI arm
--     itself is a plain cast).
--   EVENT_SUNBEAM (:425-429, init 5s-15s :391, repeat
--     10s-15s) — random player 100y (SelectTarget Random 0
--     100.0f playerOnly), DoCast(target, SPELL_SUNBEAM
--     62623), non-triggered; bridged via the
--     baltharus/maiden GetPlayersInWorld candidate-selection
--     precedent.
-- Freya KilledUnit — player-gated Talk(SAY_SLAY 2) (event 3;
-- who->GetTypeId() == TYPEID_PLAYER — the razuvious
-- player-gated variant precedent).
-- Freya DamageTaken — lethal (damage >= health) ->
-- Talk(SAY_DEATH 3) + damage = 0 (event 9; in C++ the
-- lethal branch sets damage = 0 and manually calls
-- JustDied(who), whose only bridgeable arm is the
-- Talk(SAY_DEATH) — the numeric second return performs the
-- C++-exact damage=0 rewrite (the moroes damage-rewrite
-- precedent); the companion legs have no bridges — see
-- DOCUMENTED-ONLY; the hodir consequence applies: without
-- the despawn leg, subsequent hits keep satisfying
-- damage >= health, so the death Talk re-fires on each hit
-- — verbatim C++ behavior, bounded in C++ by the 7.5s
-- despawn).
-- Elder Brightleaf Reset timers (:714-716), UpdateAI machine
-- gated on !HasAura(SPELL_DRAINED_OF_POWER 62467) (:740):
--   EVENT_SOLAR_FLARE (:756-765, init 5s-7s, repeat 5s-10s)
--     — self-cast SPELL_SOLAR_FLARE 62240, non-triggered
--     (SPELLVALUE_MAX_TARGETS mod — see DEVIATIONS).
--   EVENT_UNSTABLE_SUN_BEAM (:752-755, init 7s-12s, repeat
--     10s-15s) — me->CastSpell(me,
--     SPELL_UNSTABLE_SUN_BEAM_SUMMON 62207, true), triggered
--     self-cast.
--   EVENT_FLUX (:767-772, init 5s, repeat 7500ms) —
--     RemoveAurasDueToSpell + AddAura(SPELL_FLUX_AURA 62239)
--     (SetStackAmount — see DEVIATIONS).
-- Elder Stonebark Reset timers (:817-819), machine gated
-- :858:
--   EVENT_BARK (:870-873, init 37500ms-40s, repeat 30s-50s)
--     — DoCast(me, SPELL_PETRIFIED_BARK 62337), non-triggered
--     self-cast.
--   EVENT_FISTS (:874-877, init 25s-35s, repeat 20s-30s) —
--     DoCastVictim(SPELL_FISTS_OF_STONE 62344), non-triggered.
--   EVENT_TREMOR (:878-882, init 10s-12s, repeat 10s-20s) —
--     DoCastVictim(SPELL_GROUND_TREMOR 62325), non-triggered,
--     gated on !me->HasAura(SPELL_FISTS_OF_STONE) — C++-exact
--     via HasAura.
-- Elder Ironbranch Reset timers (:926-928), machine gated
-- :952:
--   EVENT_IMPALE (:964-967, init 18s-22s, repeat 15s-25s) —
--     DoCastVictim(SPELL_IMPALE 62310), non-triggered.
--   EVENT_THORN_SWARM (:973-976, init 7500ms-12500ms,
--     repeat 8s-13s) — DoCastVictim(SPELL_THORN_SWARM 62285),
--     non-triggered.
--   EVENT_IRON_ROOTS (:968-972, init 12s-17s :927, repeat
--     10s-20s) — random player 100y (playerOnly, aura-filter
--     -SPELL_ROOTS_IRONBRANCH), target->CastSpell(target,
--     SPELL_ROOTS_IRONBRANCH 62438, true) triggered; the
--     -SPELL_ROOTS_IRONBRANCH "without aura" filter has no
--     bridge (documented deviation), bridged otherwise via
--     the baltharus precedent.
-- Elder JustEngagedWith (all three elders, identical) —
-- Talk(SAY_ELDER_AGGRO 0) gated by
-- !me->HasAura(SPELL_DRAINED_OF_POWER 62467) (event 1; the
-- BossAI::JustEngagedWith passthrough has no bridge — the
-- auriaya engage-port precedent); the same gate is applied
-- to every elder timer handler below, mirroring the C++
-- UpdateAI early-return — C++-exact (the Drained application
-- itself lives in Freya's unbridgeable JustEngagedWith, and
-- nothing in the current engine engages elders jointly, so
-- the timers only ever run in standalone grove fights where
-- C++ carries no Drained either).
-- Elder KilledUnit (all three elders, identical) —
-- player-gated Talk(SAY_ELDER_SLAY 1) (event 3; the razuvious
-- player-gated variant precedent).
-- Elder JustDied (all three elders, identical) —
-- Talk(SAY_ELDER_DEATH 2) (event 4; the _JustDied()
-- passthrough has no bridge — the sjonnir JustDied-Talk
-- precedent).
-- npc_ancient_conservator JustEngagedWith (:1293-1296) —
-- DoCast(who, SPELL_CONSERVATOR_GRIP 62532), triggered; who
-- is the engaging unit (the event 1 target arg), bridged as
-- creature:CastSpell(target, spell, true) per the file
-- doctrine. The class's UpdateAI/summon legs stay out (see
-- DOCUMENTED-ONLY).
-- npc_iron_roots (33088) JustDied (boss_freya.cpp:243-253) —
-- me->DespawnOrUnsummon() ports (event 4, creature:Despawn,
-- bridged); the summonerGUID aura-removal legs have no bridge
-- (stay out); the rest of the class stays out (see
-- DOCUMENTED-ONLY).
--
-- DEVIATIONS from C++ (all bridge-limited, documented):
-- - No UNIT_STATE_CASTING model in Go (creature casts are
--   packet-visual), so timers fire unconditionally — the
--   maiden_of_virtue precedent documents the same gap.
-- - EVENT_SOLAR_FLARE: C++ passes SPELLVALUE_MAX_TARGETS =
--   the FLUX_AURA stack count (1-8) via CastSpellExtraArgs;
--   creature:GetStackAmount is live now (2026-10-23) but the chain
--   is broken — Lua AddAura is presence-only (stacks unreadable)
--   and creature:CastSpell has no SpellMod/extra-args path — so
--   the cast fires with the spell's default targeting.
-- - EVENT_FLUX: C++ sets the re-applied aura to
--   urand(1, 8) stacks via Flux->SetStackAmount; dropped —
--   the aura is re-applied presence-only.
-- - Freya lethal DamageTaken: cancelTimers added — C++ keeps
--   the event queue but UpdateAI stops ticking (no victim
--   after RemoveAllAttackers/AttackStop) and the creature
--   despawns 7.5s later; without the cancel the Lua timers
--   would fire through the 7.5s despawn window (the despawn
--   itself IS ported — CreateLuaEvent 7500 in freyaDamageTaken;
--   the stale "despawn leg has no bridge" wording is retracted).
--
-- DOCUMENTED-ONLY (in this header; no registrations beyond
-- entries 32906 / 32915 / 32914 / 32913):
-- Freya JustEngagedWith companion legs (:342-391) —
-- DoZoneInCombat (:345) PORTED 2026-10-23 via the live bridge
-- (C++ arm order — before the elder scan); elder scan (instance
-- GetGuidData + alive
-- checks) -> SPELL_DRAINED_OF_POWER 62467 on the elder,
-- essence casts, RemoveLootMode, elder AttackStart + threat;
-- the Talk choice (SAY_AGGRO 0 vs SAY_AGGRO_WITH_ELDER 1);
-- CastSpell SPELL_ATTUNED_TO_NATURE 62519 with 150-stack
-- spell-mod (unbridgeable) — no instance GuidData / aura /
-- threat / spell-mod bridges (RemoveLootMode IS bridged,
-- lua_objects.go:993; the arm stays out on instance-GuidData /
-- cross-creature grounds, not the loot-mode bridge).
-- Freya timer arms staying out:
--   EVENT_NATURE_BOMB (:430-433, DoCastAOE 64604 triggered)
--     — init scheduled only from the EVENT_WAVE arm after
--     waveCount > 6 (:439-444); wave-machine-gated.
--   EVENT_UNSTABLE_ENERGY (:434-438) — random target + init
--     gated on Brightleaf alive at engage (:360-365,
--     hard-mode leg).
--   EVENT_WAVE (:439-445) — SpawnWave() summon/adds leg
--     (elemental GUID bookkeeping).
--   EVENT_STRENGTHENED_IRON_ROOTS (:451-456) — random
--     target + init gated on Ironbranch alive (:372-377,
--     hard-mode leg).
--   EVENT_GROUND_TREMOR (:457-461, Talk EMOTE_GROUND_TREMOR
--     10 + DoCastAOE 62437) — init gated on Stonebark alive
--     (:366-371, hard-mode leg); blocked on the gate alone.
-- Freya JustDied companion legs (:602-~640) — chest summon
-- (summonSpell[2][4]: 62950/62952/62953/62954 10-man,
-- 62955/62956/62957/62958 25-man, indexed by
-- difficulty/elderCount) BLOCKED on the difficulty index (no
-- 10/25 raid-size bridge; creature:IsHeroic is heroic-only),
-- InterruptNonMeleeSpells, RemoveAllAttackers — PORTED 2026-10-24 into
-- freyaDamageTaken below (C++ JustDied order);
-- the live-elder shutdown sweep (RemoveAllAuras / AttackStop /
-- CombatStop / EngagementOver / DoAction(ACTION_ELDER_FREYA_KILLED))
-- — PORTED 2026-10-24 into freyaDied via the GetCreaturesInGrid elder
-- scan (no instance GuidData needed); CombatStop is bridged
-- (lua_creature_events.go, 2026-10-24) and ported into freyaDied below;
-- the elder _JustDied instance SetBossState has none — stays out. Freya's own
-- legs were PORTED 2026-10-21 into
-- freyaDamageTaken (the stale "no cast / react-state / faction
-- bridges" retraction): SetReactState REACT_PASSIVE, AttackStop,
-- SetFaction FRIENDLY, CastSpell SPELL_KNOCK_ON_WOOD_CREDIT 65074,
-- DespawnOrUnsummon(7.5s) (stale "no despawn bridge" already
-- retracted; creature:Despawn() is bridged, lua_objects.go).
-- Talk(SAY_DEATH) itself is ported via event 9.
-- Freya UpdateAI body legs — Touch-of-Eonar aura
-- maintenance (every-tick, not timer-driven); attuned-stack
-- achievement sampling; elemental 12s revive / trio-defeated
-- GUID logic; DoMeleeAttackIfReady (engine-driven in Go).
-- Freya GetData(DATA_GETTING_BACK_TO_NATURE /
-- DATA_KNOCK_ON_WOOD) — no GetData bridge (consumers: the four
-- achievements below); the elder DoAction
-- (ACTION_ELDER_FREYA_KILLED -> DespawnOrUnsummon(10s)) is PORTED
-- into freyaDied (see above).
-- Elder DamageTaken — stonebark petrified-bark reflect
-- (:841-855: who->CastSpell(who, SPELL_PETRIFIED_BARK_DMG
-- 62379) + damage = 0) PORTED as event 9 (attacker self-cast
-- via attacker:CastSpell, doomwalker precedent); only the
-- AddSpellBP0(damage) mod stays out (no extra-args bridge).
-- npc_snaplasher /
-- npc_storm_lasher / npc_ancient_water_spirit /
-- npc_detonating_lasher / npc_sun_beam / npc_nature_bomb /
-- npc_eonars_gift / npc_healthy_spore /
-- npc_unstable_sun_beam / npc_iron_roots — wave/summon
-- creatures: instance-coupled (ENSURE_AI Freya-AI callbacks,
-- waveCount from Freya's AI, GetCreature(BOSS_FREYA)),
-- random-target legs (tidal wave, stormbolt, nature fury),
-- threat/AttackStart legs, summon/despawn/RemoveFromWorld/
-- DisappearAndDie legs, SetReactState / SetFlag /
-- SetImmuneToPC / SetCombatMovement legs,
-- FindNearestGameObject leg — no spawn / movement /
-- threat bridges; no registrations. (npc_ancient_conservator
-- is registered for its engage arm only — see PORTED ARMS;
-- its Reset/UpdateAI/summon legs stay out for the same
-- reasons.)
-- spell_freya_attuned_to_nature_dose_reduction /
-- spell_freya_iron_roots — no SpellScript bridges; both join
-- the no-SpellScript-bridge queue.
-- achievement_getting_back_to_nature /
-- achievement_knock_on_wood / achievement_knock_knock_on_wood /
-- achievement_knock_knock_knock_on_wood (OnCheck: GetData
-- legs) — no GetData / achievement bridges; all four join
-- the unmodeled-achievement queue.

local ENTRY_FREYA = 32906
local ENTRY_BRIGHTLEAF = 32915
local ENTRY_STONEBARK = 32914
local ENTRY_IRONBRANCH = 32913
local ENTRY_ANCIENT_CONSERVATOR = 33203
local ENTRY_IRON_ROOTS = 33088 -- boss_freya.cpp:168

local SPELL_DRAINED_OF_POWER = 62467

-- Freya
local SPELL_ENRAGE = 47008
local SPELL_SUMMON_EONAR_GIFT = 62572
local SPELL_SUNBEAM = 62623
local SPELL_FREYA_UNSTABLE_SUNBEAM = 62450
local SPELL_FREYA_GROUND_TREMOR = 62437
local SPELL_ROOTS_FREYA = 62283

-- Elder Brightleaf
local SPELL_UNSTABLE_SUN_BEAM_SUMMON = 62207
local SPELL_SOLAR_FLARE = 62240
local SPELL_FLUX_AURA = 62239

-- Elder Stonebark
local SPELL_PETRIFIED_BARK = 62337
local SPELL_PETRIFIED_BARK_DMG = 62379
local SPELL_FISTS_OF_STONE = 62344
local SPELL_GROUND_TREMOR = 62325

-- Elder Ironbranch
local SPELL_IMPALE = 62310
local SPELL_THORN_SWARM = 62285
local SPELL_ROOTS_IRONBRANCH = 62438

-- Ancient Conservator
local SPELL_CONSERVATOR_GRIP = 62532

local SAY_SLAY = 2
local SAY_DEATH = 3
local SAY_BERSERK = 4
local SAY_AGGRO = 0 -- boss_freya.cpp:32
local SAY_AGGRO_WITH_ELDER = 1 -- boss_freya.cpp:33
local EMOTE_LIFEBINDERS_GIFT = 8
local EMOTE_GROUND_TREMOR = 10
local EMOTE_IRON_ROOTS = 11

local SAY_ELDER_AGGRO = 0
local SAY_ELDER_SLAY = 1
local SAY_ELDER_DEATH = 2

local SPELL_KNOCK_ON_WOOD_CREDIT = 65074 -- boss_freya.cpp:92
local FACTION_FRIENDLY = 35 -- SharedDefines.h:231
local REACT_PASSIVE = 0 -- UnitDefines.h:340

-- GUID-keyed timer harness (the maiden_of_virtue precedent):
-- CreateLuaEvent ids per creature GUID, cancelled on
-- OnLeaveCombat / OnDied / OnReset. A {min, max} delay is a
-- randtime range, matching C++ ScheduleEvent(min, max).
local timers = {}

local freyaFinished = {}

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

-- C++ elder UpdateAI: if (!UpdateVictim() ||
-- me->HasAura(SPELL_DRAINED_OF_POWER)) return; — the victim
-- half is modeled by timer lifetime (cancelled on
-- leave-combat/death/reset); the aura half is C++-exact via
-- HasAura (the same gate the aggro Talk already used).
local function elderTimersActive(creature)
    return not creature:HasAura(SPELL_DRAINED_OF_POWER)
end

-- C++ SelectTarget(SelectTargetMethod::Random, 0, dist,
-- playerOnly) legs (the baltharus GetPlayersInWorld
-- candidate-selection precedent; thorim this run).
local function randomPlayerInRange(creature, minDist, maxDist, excludeAura)
    local mapId, instanceId = creature:GetMapId(), creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead()
                and (excludeAura == nil or not plr:HasAura(excludeAura)) then
            local dist = creature:GetDistance(plr)
            if dist >= minDist and dist <= maxDist then
                candidates[#candidates + 1] = plr
            end
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_EONAR_GIFT (boss_freya.cpp:446-449):
-- Talk(EMOTE_LIFEBINDERS_GIFT) + DoCast(me,
-- SPELL_SUMMON_EONAR_GIFT), non-triggered self-cast;
-- init 25s (:389), repeat 40s-50s.
local function onEonarGift(creature, guid)
    creature:Talk(EMOTE_LIFEBINDERS_GIFT)
    creature:CastSpell(creature, SPELL_SUMMON_EONAR_GIFT)
    schedule(guid, "eonar_gift", {40000, 50000}, function() onEonarGift(creature, guid) end)
end

-- C++ JustEngagedWith (:388-391) schedules the timers, and the aggro
-- Talk choice (SAY_AGGRO vs SAY_AGGRO_WITH_ELDER, :379-382) IS ported
-- below: elderCount (alive elders via instance GuidData — no bridge)
-- is reconstructed via GetCreaturesInGrid + IsAlive over the three
-- elder entries (the C++ counts Elder[n] && Elder[n]->IsAlive(),
-- :357). Only the unconditional timer arms register here otherwise.
-- C++ EVENT_SUNBEAM (boss_freya.cpp:425-429): random player
-- 100y, DoCast(target, SPELL_SUNBEAM), 2-arg = non-triggered;
-- init 5s-15s (:391), repeat 10s-15s.
local function onSunbeam(creature, guid)
    local target = randomPlayerInRange(creature, 0, 100)
    if target then
        creature:CastSpell(target, SPELL_SUNBEAM)
    end
    schedule(guid, "sunbeam", {10000, 15000}, function() onSunbeam(creature, guid) end)
end

-- C++ EVENT_UNSTABLE_ENERGY (boss_freya.cpp:434-437): random
-- player 100y, DoCast(target, SPELL_FREYA_UNSTABLE_SUNBEAM 62450,
-- true) triggered; init 10s-20s (:364), repeat 15s-20s. Gated on
-- Brightleaf alive at engage (see freyaEnterCombat).
local function onUnstableEnergy(creature, guid)
    local target = randomPlayerInRange(creature, 0, 100)
    if target then
        creature:CastSpell(target, SPELL_FREYA_UNSTABLE_SUNBEAM, true)
    end
    schedule(guid, "unstable_energy", {15000, 20000}, function() onUnstableEnergy(creature, guid) end)
end

-- C++ EVENT_GROUND_TREMOR (boss_freya.cpp:457-460): Talk
-- (EMOTE_GROUND_TREMOR 10) + DoCastAOE (self-cast)
-- SPELL_FREYA_GROUND_TREMOR 62437, non-triggered; init 10s-20s
-- (:370), repeat 25s-28s. Gated on Stonebark alive at engage.
local function onFreyaGroundTremor(creature, guid)
    creature:Talk(EMOTE_GROUND_TREMOR)
    creature:CastSpell(creature, SPELL_FREYA_GROUND_TREMOR)
    schedule(guid, "freya_ground_tremor", {25000, 28000}, function() onFreyaGroundTremor(creature, guid) end)
end

-- C++ EVENT_STRENGTHENED_IRON_ROOTS (boss_freya.cpp:451-455):
-- Talk(EMOTE_IRON_ROOTS 11) + random player 100y without
-- SPELL_ROOTS_FREYA -> target self-cast 62283, triggered; init
-- 10s-20s (:376), repeat 12s-20s. Gated on Ironbranch alive at
-- engage.
local function onStrengthenedIronRoots(creature, guid)
    creature:Talk(EMOTE_IRON_ROOTS)
    local target = randomPlayerInRange(creature, 0, 100, SPELL_ROOTS_FREYA)
    if target then
        target:CastSpell(target, SPELL_ROOTS_FREYA, true)
    end
    schedule(guid, "strengthened_iron_roots", {12000, 20000}, function() onStrengthenedIronRoots(creature, guid) end)
end

local function freyaEnterCombat(event, creature, target)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    -- C++ :345 — me-form DoZoneInCombat after BossAI::JustEngagedWith,
    -- before the elder scan (C++ arm order).
    creature:DoZoneInCombat()
    -- C++ JustEngagedWith (boss_freya.cpp:379-382): elderCount == 0 ->
    -- Talk(SAY_AGGRO), else Talk(SAY_AGGRO_WITH_ELDER); elderCount is
    -- the alive-elder count (C++: Elder[n] && Elder[n]->IsAlive(),
    -- :357) reconstructed via GetCreaturesInGrid + IsAlive.
    local eldersAlive = 0
    local elderAlive = {}
    for i, entry in ipairs({ ENTRY_BRIGHTLEAF, ENTRY_STONEBARK, ENTRY_IRONBRANCH }) do
        elderAlive[i] = false
        for _, elder in ipairs(creature:GetCreaturesInGrid(entry, 150)) do
            if elder:IsAlive() then
                eldersAlive = eldersAlive + 1
                elderAlive[i] = true
            end
        end
    end
    if eldersAlive == 0 then
        creature:Talk(SAY_AGGRO)
    else
        creature:Talk(SAY_AGGRO_WITH_ELDER)
    end
    -- C++ EVENT_ENRAGE (:421-424): one-shot 10min (:390);
    -- Talk(SAY_BERSERK) + DoCast(me, SPELL_ENRAGE),
    -- non-triggered self-cast; no re-arm in C++.
    schedule(guid, "enrage", 600000, function()
        creature:Talk(SAY_BERSERK)
        creature:CastSpell(creature, SPELL_ENRAGE)
    end)
    schedule(guid, "eonar_gift", 25000, function() onEonarGift(creature, guid) end)
    schedule(guid, "sunbeam", {5000, 15000}, function() onSunbeam(creature, guid) end)
    -- C++ JustEngagedWith elder-gated arms (:362-377): the gate is
    -- the same per-entry alive scan as the Talk choice above
    -- (C++ reads instance GuidData; the grid scan is this file's
    -- engage-time precedent). Inits are all 10s-20s.
    if elderAlive[1] then
        schedule(guid, "unstable_energy", {10000, 20000}, function() onUnstableEnergy(creature, guid) end)
    end
    if elderAlive[2] then
        schedule(guid, "freya_ground_tremor", {10000, 20000}, function() onFreyaGroundTremor(creature, guid) end)
    end
    if elderAlive[3] then
        schedule(guid, "strengthened_iron_roots", {10000, 20000}, function() onStrengthenedIronRoots(creature, guid) end)
    end
end

local function freyaLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ Freya JustDied elder shutdown sweep (boss_freya.cpp:630-641):
-- for each live elder (instance GuidData in C++, the GetCreaturesInGrid
-- scan here — the engage-time precedent above): RemoveAllAuras +
-- CombatStop(true) (AttackStop + RemoveAllAttackers + EndAllPvECombat
-- legs, bridged 2026-10-24) + the elder DoAction (DespawnOrUnsummon 10s,
-- fire-and-forget like the 7.5s self-despawn above, + the elder's
-- events.Reset as cancelTimers). The elder _JustDied instance SetBossState
-- has none; EngagementOver is freya-side housekeeping on the dying boss
-- (its Go analog, InCombat=false, rides CombatStop). Both stay out.
local function freyaElderShutdown(creature)
    for _, entry in ipairs({ ENTRY_BRIGHTLEAF, ENTRY_STONEBARK, ENTRY_IRONBRANCH }) do
        for _, elder in ipairs(creature:GetCreaturesInGrid(entry, 150)) do
            if elder:IsAlive() then
                elder:RemoveAllAuras()
                elder:CombatStop(true)
                cancelTimers(elder:GetGUID())
                CreateLuaEvent(function()
                    elder:Despawn()
                end, 10000)
            end
        end
    end
end

-- Fallback for true-kill paths: on the lethal DamageTaken path the
-- kill funnel never runs (damage is rewritten to 0), so the sweep
-- is driven from freyaDamageTaken below; this stays as the
-- event-4 fallback.
local function freyaDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    freyaElderShutdown(creature)
end

local function freyaReset(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_SLAY) — the razuvious player-gated variant
-- precedent.
local function freyaTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

-- C++ DamageTaken: damage >= me->GetHealth() -> damage = 0
-- + manual JustDied(who) call; the manual JustDied's bridgeable
-- arms are Talk(SAY_DEATH) + DespawnOrUnsummon(7500ms)
-- (boss_freya.cpp:626) + the 2026-10-21 completion: SetReactState
-- REACT_PASSIVE, AttackStop, SetFaction(FACTION_FRIENDLY), and
-- CastSpell(me, SPELL_KNOCK_ON_WOOD_CREDIT 65074, true) as
-- creature:CastSpell(creature, spell, true) — the stale "no cast /
-- react-state / faction bridges" header claim retracted. The
-- numeric second return performs the C++-exact damage=0 rewrite
-- (the moroes damage-rewrite precedent; the hodir consequence
-- applies). cancelTimers models the C++ post-lethal stop (no
-- victim after RemoveAllAttackers/AttackStop; without the cancel
-- the Lua timers would fire forever on the unkillable body). The
-- 7.5s despawn is a fire-and-forget one-shot CreateLuaEvent —
-- C++-exact: C++ DespawnOrUnsummon rides the creature's own
-- despawn timer, not the EventMap, so it is deliberately NOT
-- tracked in the timers table (leave-combat/reset must not
-- cancel it, which C++ never does either). Still out: the chest
-- summon (summonSpell indexed by GetMap()->GetDifficulty() —
-- no 10/25 raid-size bridge; creature:IsHeroic is heroic-only).
-- InterruptNonMeleeSpells (no-op twin) and RemoveAllAttackers are ported
-- above in C++ JustDied order. The live-elder shutdown sweep runs
-- here, after the credit cast (C++ JustDied order: self legs, then
-- the elder sweep :630-641) — event 4 never fires on this path
-- because the damage rewrite keeps health above 0. The GUID-keyed
-- freyaFinished latch mirrors C++ JustDied's _encounterFinished
-- early-return (:602-606): later lethal hits only re-zero damage.
-- C++ Reset also early-returns when finished (:318-322), so the
-- latch is deliberately NOT cleared on Reset; a respawned Freya is
-- a new GUID.
local function freyaDamageTaken(event, creature, attacker, damage)
    local guid = creature:GetGUID()
    if damage >= creature:GetHealth() then
        if freyaFinished[guid] then
            return false, 0
        end
        freyaFinished[guid] = true
        creature:Talk(SAY_DEATH)
        creature:SetReactState(REACT_PASSIVE)
        creature:InterruptNonMeleeSpells()
        creature:RemoveAllAttackers()
        creature:AttackStop()
        creature:SetFaction(FACTION_FRIENDLY)
        cancelTimers(guid)
        CreateLuaEvent(function()
            creature:Despawn()
        end, 7500)
        creature:CastSpell(creature, SPELL_KNOCK_ON_WOOD_CREDIT, true)
        freyaElderShutdown(creature)
        return false, 0
    end
end

-- C++ JustEngagedWith (BossAI passthrough): Talk(SAY_ELDER_AGGRO)
-- unless the elder still carries SPELL_DRAINED_OF_POWER —
-- the auriaya engage-port precedent with the C++-exact aura
-- gate. C++ Reset() (:714-716 / :817-819 / :926-928)
-- schedules the elder timers; they start ticking on combat
-- here, matching the C++ UpdateAI (which early-returns
-- without a victim).
local function elderAggroTalk(creature)
    if not creature:HasAura(SPELL_DRAINED_OF_POWER) then
        creature:Talk(SAY_ELDER_AGGRO)
    end
end

-- Elder Brightleaf (:714-716, machine gated :740).
-- C++ EVENT_SOLAR_FLARE (:756-765): init 5s-7s, repeat
-- 5s-10s; me->CastSpell(me, SPELL_SOLAR_FLARE, args),
-- non-triggered self-cast — the SPELLVALUE_MAX_TARGETS =
-- flux-stack-count mod has no bridge (see header).
local function onSolarFlare(creature, guid)
    if elderTimersActive(creature) then
        creature:CastSpell(creature, SPELL_SOLAR_FLARE)
    end
    schedule(guid, "solar_flare", {5000, 10000}, function() onSolarFlare(creature, guid) end)
end

-- C++ EVENT_UNSTABLE_SUN_BEAM (:752-755): init 7s-12s,
-- repeat 10s-15s; me->CastSpell(me,
-- SPELL_UNSTABLE_SUN_BEAM_SUMMON, true) — triggered
-- self-cast.
local function onUnstableSunBeam(creature, guid)
    if elderTimersActive(creature) then
        creature:CastSpell(creature, SPELL_UNSTABLE_SUN_BEAM_SUMMON, true)
    end
    schedule(guid, "unstable_sun_beam", {10000, 15000}, function() onUnstableSunBeam(creature, guid) end)
end

-- C++ EVENT_FLUX (:767-772): init 5s, repeat 7500ms;
-- RemoveAurasDueToSpell + AddAura(SPELL_FLUX_AURA) —
-- SetStackAmount(urand(1, 8)) has no bridge (only the read side,
-- creature:GetStackAmount, was built 2026-10-23; the Go aura
-- model is presence-only), so the aura is re-applied as-is.
local function onFlux(creature, guid)
    if elderTimersActive(creature) then
        creature:RemoveAura(SPELL_FLUX_AURA)
        creature:AddAura(SPELL_FLUX_AURA)
    end
    schedule(guid, "flux", 7500, function() onFlux(creature, guid) end)
end

local function brightleafEnterCombat(event, creature, target)
    elderAggroTalk(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "solar_flare", {5000, 7000}, function() onSolarFlare(creature, guid) end)
    schedule(guid, "unstable_sun_beam", {7000, 12000}, function() onUnstableSunBeam(creature, guid) end)
    schedule(guid, "flux", 5000, function() onFlux(creature, guid) end)
end

-- Elder Stonebark (:817-819, machine gated :858).
-- C++ EVENT_BARK (:870-873): init 37500ms-40s, repeat
-- 30s-50s; DoCast(me, SPELL_PETRIFIED_BARK), non-triggered
-- self-cast.
local function onBark(creature, guid)
    if elderTimersActive(creature) then
        creature:CastSpell(creature, SPELL_PETRIFIED_BARK)
    end
    schedule(guid, "bark", {30000, 50000}, function() onBark(creature, guid) end)
end

-- C++ EVENT_FISTS (:874-877): init 25s-35s, repeat 20s-30s;
-- DoCastVictim(SPELL_FISTS_OF_STONE), non-triggered.
local function onFists(creature, guid)
    if elderTimersActive(creature) then
        creature:CastSpell(nil, SPELL_FISTS_OF_STONE)
    end
    schedule(guid, "fists", {20000, 30000}, function() onFists(creature, guid) end)
end

-- C++ EVENT_TREMOR (:878-882): init 10s-12s, repeat
-- 10s-20s; if (!me->HasAura(SPELL_FISTS_OF_STONE))
-- DoCastVictim(SPELL_GROUND_TREMOR), non-triggered.
local function onTremor(creature, guid)
    if elderTimersActive(creature) and not creature:HasAura(SPELL_FISTS_OF_STONE) then
        creature:CastSpell(nil, SPELL_GROUND_TREMOR)
    end
    schedule(guid, "tremor", {10000, 20000}, function() onTremor(creature, guid) end)
end

local function stonebarkEnterCombat(event, creature, target)
    elderAggroTalk(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "tremor", {10000, 12000}, function() onTremor(creature, guid) end)
    schedule(guid, "fists", {25000, 35000}, function() onFists(creature, guid) end)
    schedule(guid, "bark", {37500, 40000}, function() onBark(creature, guid) end)
end

-- Elder Ironbranch (:926-928, machine gated :952).
-- C++ EVENT_IMPALE (:964-967): init 18s-22s, repeat
-- 15s-25s; DoCastVictim(SPELL_IMPALE), non-triggered.
local function onImpale(creature, guid)
    if elderTimersActive(creature) then
        creature:CastSpell(nil, SPELL_IMPALE)
    end
    schedule(guid, "impale", {15000, 25000}, function() onImpale(creature, guid) end)
end

-- C++ EVENT_THORN_SWARM (:973-976): init 7500ms-12500ms,
-- repeat 8s-13s; DoCastVictim(SPELL_THORN_SWARM),
-- non-triggered.
local function onThornSwarm(creature, guid)
    if elderTimersActive(creature) then
        creature:CastSpell(nil, SPELL_THORN_SWARM)
    end
    schedule(guid, "thorn_swarm", {8000, 13000}, function() onThornSwarm(creature, guid) end)
end

-- C++ EVENT_IRON_ROOTS (boss_freya.cpp:968-972): random
-- player 100y (playerOnly, -SPELL_ROOTS_IRONBRANCH
-- aura-filter), target->CastSpell(target,
-- SPELL_ROOTS_IRONBRANCH 62438, true) — triggered;
-- init 12s-17s (:927), repeat 10s-20s. The aura filter rides
-- randomPlayerInRange's excludeAura (player:HasAura); the
-- target self-casts (player:CastSpell, vezax precedent).
local function onIronRoots(creature, guid)
    if elderTimersActive(creature) then
        local target = randomPlayerInRange(creature, 0, 100, SPELL_ROOTS_IRONBRANCH)
        if target then
            target:CastSpell(target, SPELL_ROOTS_IRONBRANCH, true)
        end
    end
    schedule(guid, "iron_roots", {10000, 20000}, function() onIronRoots(creature, guid) end)
end

local function ironbranchEnterCombat(event, creature, target)
    elderAggroTalk(creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "impale", {18000, 22000}, function() onImpale(creature, guid) end)
    schedule(guid, "thorn_swarm", {7500, 12500}, function() onThornSwarm(creature, guid) end)
    schedule(guid, "iron_roots", {12000, 17000}, function() onIronRoots(creature, guid) end)
end

local function elderLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ elder Reset (:709-713 / :812-816 / :921-925): if
-- (me->HasAura(SPELL_DRAINED_OF_POWER)) me->
-- RemoveAurasDueToSpell(SPELL_DRAINED_OF_POWER) — creature:HasAura
-- + creature:RemoveAura (moorabi precedent); shared by all three
-- elders (one handler, three registrations).
local function elderReset(event, creature)
    cancelTimers(creature:GetGUID())
    if creature:HasAura(SPELL_DRAINED_OF_POWER) then
        creature:RemoveAura(SPELL_DRAINED_OF_POWER)
    end
end

-- C++ KilledUnit: who->GetTypeId() == TYPEID_PLAYER, then
-- Talk(SAY_ELDER_SLAY) — the razuvious player-gated variant
-- precedent.
local function elderTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_ELDER_SLAY)
    end
end

-- C++ JustDied: _JustDied() passthrough, then
-- Talk(SAY_ELDER_DEATH) — the sjonnir JustDied-Talk
-- precedent.
local function elderDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_ELDER_DEATH)
end

-- C++ boss_stonebarkAI::DamageTaken (boss_freya.cpp:841-855): if
-- the attacker exists, isn't self (GUID compare, malchezaar
-- precedent), and Stonebark carries SPELL_PETRIFIED_BARK, the
-- attacker self-casts SPELL_PETRIFIED_BARK_DMG 62379, triggered,
-- and damage = 0. Only the AddSpellBP0(damage) mod stays out
-- (no extra-args bridge).
local function stonebarkDamageTaken(event, creature, attacker, damage)
    if attacker and attacker:GetGUID() ~= creature:GetGUID()
            and creature:HasAura(SPELL_PETRIFIED_BARK) then
        attacker:CastSpell(attacker, SPELL_PETRIFIED_BARK_DMG, true)
        return false, 0
    end
end

RegisterCreatureEvent(ENTRY_FREYA, 1, freyaEnterCombat)
RegisterCreatureEvent(ENTRY_FREYA, 2, freyaLeaveCombat)
RegisterCreatureEvent(ENTRY_FREYA, 3, freyaTargetDied)
RegisterCreatureEvent(ENTRY_FREYA, 4, freyaDied)
RegisterCreatureEvent(ENTRY_FREYA, 9, freyaDamageTaken)
RegisterCreatureEvent(ENTRY_FREYA, 23, freyaReset)

RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 1, brightleafEnterCombat)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 2, elderLeaveCombat)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 3, elderTargetDied)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 4, elderDied)
RegisterCreatureEvent(ENTRY_BRIGHTLEAF, 23, elderReset)

RegisterCreatureEvent(ENTRY_STONEBARK, 1, stonebarkEnterCombat)
RegisterCreatureEvent(ENTRY_STONEBARK, 2, elderLeaveCombat)
RegisterCreatureEvent(ENTRY_STONEBARK, 3, elderTargetDied)
RegisterCreatureEvent(ENTRY_STONEBARK, 4, elderDied)
RegisterCreatureEvent(ENTRY_STONEBARK, 9, stonebarkDamageTaken)
RegisterCreatureEvent(ENTRY_STONEBARK, 23, elderReset)

RegisterCreatureEvent(ENTRY_IRONBRANCH, 1, ironbranchEnterCombat)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 2, elderLeaveCombat)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 3, elderTargetDied)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 4, elderDied)
RegisterCreatureEvent(ENTRY_IRONBRANCH, 23, elderReset)

-- C++ npc_ancient_conservator::JustEngagedWith
-- (boss_freya.cpp:1293-1296): DoCast(who, SPELL_CONSERVATOR_GRIP
-- 62532), triggered — who is the engaging unit, bridged as the
-- event 1 target arg (creature:CastSpell(target, spell, true)
-- per the file doctrine).
local function conservatorEnterCombat(event, creature, target)
    creature:CastSpell(target, SPELL_CONSERVATOR_GRIP, true)
end

RegisterCreatureEvent(ENTRY_ANCIENT_CONSERVATOR, 1, conservatorEnterCombat)

-- C++ npc_iron_rootsAI::JustDied (boss_freya.cpp:243-253): the
-- summoner aura-removal legs ride the summonerGUID member (no
-- bridge, stay out); me->DespawnOrUnsummon() ports
-- (creature:Despawn, bridged).
local function ironRootsJustDied(event, creature, killer)
    creature:Despawn()
end

RegisterCreatureEvent(ENTRY_IRON_ROOTS, 4, ironRootsJustDied)
