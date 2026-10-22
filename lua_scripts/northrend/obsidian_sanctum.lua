-- Tenebron / Shadron / Vesperon (Obsidian Sanctum drake mini-bosses) —
-- Lua port of
-- src/server/scripts/Northrend/ChamberOfAspects/ObsidianSanctum/obsidian_sanctum.cpp
-- (npc_tenebron / npc_shadron / npc_vesperon (CreatureScripts), each via
-- GetObsidianSanctumAI<...AI> (dummy_dragonAI : ScriptedAI);
-- registered from inside AddSC_obsidian_sanctum(); loader decl 95 /
-- call 290 per northrend_script_loader.cpp — the SECOND group of the
-- "// Obsidian Sanctum" block in AddNorthrendScripts(), immediately
-- after AddSC_boss_sartharion() (decl 94 / call 289), under the
-- "// Obsidian Sanctum" marker; the call after it is
-- AddSC_instance_obsidian_sanctum() (decl 96 / call 291) — loader
-- order confirmed this run; the checkpoint sequence
-- (boss_sartharion -> obsidian_sanctum) is followed).
-- Entries: 30452 Tenebron (obsidian_sanctum.h NPC_TENEBRON, line 41),
-- 30451 Shadron (NPC_SHADRON, line 42), 30449 Vesperon
-- (NPC_VESPERON, line 43) — entry-verifiable (the nexus_commanders /
-- malygos / sartharion kalecgos precedent); the CreatureScript
-- ScriptName bindings are DB-side as usual.
-- Sole-source verified: whole-server-tree grep for each of the
-- twelve script names registered from AddSC_obsidian_sanctum()
-- hits obsidian_sanctum.cpp only; zero sql/ hits. No obsidian_sanctum
-- lua existed.
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 3
-- OnTargetDied, 4 OnDied, 23 OnReset. Timers ride the CreateLuaEvent
-- convention (baltharus / maiden precedent): GUID-keyed timers
-- table, cancel on events 2/4/23; melee is engine-driven in Go
-- (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++-exact for all modeled arms; dummy_dragonAI base,
-- shared across the three drakes):
-- JustEngagedWith — Talk(SAY_AGGRO 0) + the shadow-breath and
-- shadow-fissure timers (event 1; the DoZoneInCombat leg has no
-- bridge — the tharon_ja / malygos DoZoneInCombat precedent; the
-- EVENT_SHADOW_FISSURE leg rides the fissureTick port, so the old
-- "no target-selection" leg claim is retracted).
-- EVENT_SHADOW_BREATH — Talk(SAY_BREATH 3) +
-- DoCastVictim(SPELL_SHADOW_BREATH 57570) 2-arg non-triggered,
-- 20s init / {20s,25s} re-arm (:193 / :392-:396).
-- EVENT_SHADOW_FISSURE — SelectTarget(Random, 0, 0.0f, true)
-- (playerOnly=true, idiom-exact) -> randomPlayerInRange(creature, 0,
-- 500), nil-guarded pick, DoCast 2-arg non-triggered
-- (SPELL_SHADOW_FISSURE 57579), {15s,20s} unconditional re-arm,
-- 5s init (:192 / :387-:391; the old "no target-selection / cast
-- bridges" claim was stale).
-- KilledUnit — C++-gated on who->GetTypeId() == TYPEID_PLAYER,
-- then Talk(SAY_SLAY 1) — PORTED (event 3; the razuvious
-- player-gated variant precedent: victim nil-guard,
-- victim:GetObjectType() == "Player").
-- JustDied — Talk(SAY_DEATH 2) + the entry-switched SELF
-- RemoveAurasDueToSpell leg (SPELL_POWER_OF_TENEBRON 61248 /
-- SPELL_POWER_OF_SHADRON 58105 / SPELL_POWER_OF_VESPERON 61251 via
-- creature:RemoveAura — stale "only the Talk arm is bridgeable" claim
-- retracted) PORTED (event 4; the _canLoot / SetLootRecipient leg, the
-- DoRemoveAurasDueToSpellOnPlayers player-loop leg, the acolyte
-- KillSelf legs (FindNearestCreature NPC_ACOLYTE_OF_SHADRON 31218 /
-- NPC_ACOLYTE_OF_VESPERON 31219, 100.0f), the SetBossState
-- (DATA_TENEBRON / DATA_SHADRON / DATA_VESPERON, DONE) legs, and
-- the Twilight Revenge on Sartharion (SPELL_TWILIGHT_REVENGE 60639
-- DoCast via instance GUID fetch) have no loot / player-loop / kill /
-- instance / cast bridges — documented-only).
-- DOCUMENTED-ONLY (in this header; no registration beyond entries
-- 30452 / 30451 / 30449 — no bridgeable arms):
-- dummy_dragonAI Reset (flag clear + events.Reset + Initialize —
-- no bridge); SetData DATA_CAN_LOOT (_canLoot — no bridge);
-- MovementInform (the POINT_ID_INIT 100 / POINT_ID_LAND 200
-- waypoint machine over dragonCommon[6]: instance-gated
-- EnterEvadeMode, Clear + DoZoneInCombat + random-target chase
-- legs at the LAND point, waypointId wrap + EVENT_FREE_MOVEMENT
-- — no motion / instance / target-selection bridges);
-- OpenPortal (portal grid search (GO_TWILIGHT_PORTAL 193988,
-- 50.0f), Tenebron egg summons (NPC_TWILIGHT_EGG 30882 /
-- NPC_SARTHARION_TWILIGHT_EGG 31204, six positions,
-- TEMPSUMMON_CORPSE_TIMED_DESPAWN 20s), Shadron / Vesperon
-- acolyte summons (NPC_ACOLYTE_OF_SHADRON 31218 /
-- NPC_ACOLYTE_OF_VESPERON 31219, two position sets per drake),
-- the Vesperon InterruptNonMeleeSpells + 32747 cast leg, the
-- Talk(WHISPER_OPEN_PORTAL 6 / WHISPER_OPENED_PORTAL 7) legs
-- (portal-entry Talk — no bridgeable combat event), the
-- portal SetRespawnTime(30000) leg — no summon / cast /
-- GO-respawn bridges (creature-search IS bridged —
-- lua_creature_events.go:1204 — but the arm is GO-gated);
-- the UpdateAI EVENT_FREE_MOVEMENT machine (no motion bridge).
-- npc_tenebron — Reset (base); JustEngagedWith (base +
-- ScheduleEvent EVENT_HATCH_EGGS 30s); UpdateAI EVENT_HATCH_EGGS ->
-- OpenPortal (obsidian_sanctum.cpp:451-453) — the timer bridge is
-- live (CreateLuaEvent), but the payload is OpenPortal only:
-- instance-gated egg summons (no summon / GO-grid-search /
-- instance bridges — GO grid search genuinely absent from the GO
-- surface; creature GetCreaturesInGrid is bridged but the arm is
-- GO-gated), so the timer arm stays out (the stale
-- "no timer bridge" claim is re-keyed: payload-unbridged, not
-- timer-unbridged).
-- npc_shadron — Reset (base + the self HasAura-gated aura strips
-- SPELL_TWILIGHT_TORMENT_VESP 57948 / SPELL_GIFT_OF_TWILIGTH_SHA
-- 57835 PORTED — stale "no aura bridge" claim retracted;
-- SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) has no instance bridge —
-- documented); JustEngagedWith (base + ScheduleEvent
-- EVENT_ACOLYTE_SHADRON 1min); UpdateAI EVENT_ACOLYTE_SHADRON
-- (obsidian_sanctum.cpp:519-531: instance-gated OpenPortal +
-- SetBossState(DATA_PORTAL_OPEN, IN_PROGRESS)) — all payload legs
-- unbridged (no summon / instance bridges); timer arm stays out.
-- npc_vesperon — Reset (base); JustEngagedWith (base +
-- ScheduleEvent EVENT_ACOLYTE_VESPERON 1min); UpdateAI
-- EVENT_ACOLYTE_VESPERON (obsidian_sanctum.cpp:590-597:
-- instance-gated OpenPortal + DoCastVictim(SPELL_TWILIGHT_TORMENT_VESP
-- 57948), 60s/70s re-arm) — PORTED this run (tormentTick): the
-- victim-cast leg rides the live timer and CastSpell(nil, spell)
-- bridges. Documented deviation: the DATA_PORTAL_OPEN IN_PROGRESS
-- gate and the OpenPortal summon legs have no instance / summon
-- bridges, so in Lua the cast fires every 60-70s while the drake
-- lives instead of being skipped by the instance gate.
-- npc_acolyte_of_shadron (NPC_ACOLYTE_OF_SHADRON 31218,
-- entry-verifiable from the local enum) — Reset self
-- AddAura(SPELL_TWILIGHT_SHIFT_ENTER 57620) PORTED (the aura leg
-- is bridged); DespawnOrUnsummon(28s) is inexpressible (Go
-- Despawn takes no delay arg); the instance-gated AddAura
-- gift-of-twilight 58766/57835 legs and JustDied's instance
-- SetBossState + player-loop cast/remove legs have no instance
-- bridges — documented-only.
-- npc_acolyte_of_vesperon (NPC_ACOLYTE_OF_VESPERON 31219) —
-- Reset self AddAura(SPELL_TWILIGHT_SHIFT_ENTER 57620) +
-- DoCast(me, SPELL_TWILIGHT_TORMENT_VESP_ACO 58853) (2-arg
-- non-triggered self-cast) PORTED; JustDied self
-- RemoveAurasDueToSpell(58853) PORTED (the vesperon/player/instance
-- legs have no bridges — documented-only); UpdateAI melee.
-- npc_twilight_eggs (NPC_TWILIGHT_EGG 30882 /
-- NPC_SARTHARION_TWILIGHT_EGG 31204) — Reset self
-- AddAura(SPELL_TWILIGHT_SHIFT_ENTER 57620) PORTED; the
-- EVENT_TWILIGHT_EGGS 20s payload (SetBossState + SpawnWhelps whelp
-- summons NPC_TWILIGHT_WHELP 30890 / NPC_SARTHARION_TWILIGHT_WHELP
-- 31214 + KillSelf) and JustSummoned (DoZoneInCombat) have no
-- summon / instance bridges — documented-only.
-- npc_flame_tsunami (NPC_FLAME_TSUNAMI 30616, boss_sartharion.cpp
-- enum) — EVENT_TSUNAMI_TIMER DoCast(me,
-- SPELL_FLAME_TSUNAMI_DMG_AURA 57491) (2-arg non-triggered
-- self-cast, 100ms init / 500ms re-arm) PORTED via CreateLuaEvent
-- (seeded on Reset, cancelled on event 37 — the wave is stationary;
-- the C++ leap motion has no bridge); Reset SetReactState(REACT_PASSIVE)
-- + SetFlag(NON_ATTACKABLE | NOT_SELECTABLE) PORTED (bridged);
-- EVENT_TSUNAMI_BUFF (nearest live NPC_LAVA_BLAZE 30643 within 10yd
-- self-casts SPELL_FLAME_TSUNAMI_BUFF 60430, triggered, 1s/1s)
-- PORTED via the FindNearestCreature bridge; the ctor legs
-- (SetDisplayId 11686 + aura SPELL_FLAME_TSUNAMI 57494) are PORTED
-- via event 5 (OnSpawn) — the stale "no spawn / display bridges"
-- claim retracted 2026-10-21.
-- npc_twilight_fissure — entry UNVERIFIABLE from C++ (no local
-- NPC enum; the fissure is spawned by SPELL_SHADOW_FISSURE
-- 57579, entry not named in the C++ tree) — joins the
-- entry-unverifiable queue (the npc_image_belgaristrasz precedent);
-- Reset / UpdateAI (DoCastAOE(SPELL_VOID_BLAST 57581) +
-- RemoveAllAuras + KillSelf): AddAura / RemoveAllAuras / CastSpell
-- are bridged (stale "no aura / cast bridges" claim retracted),
-- but the AI cannot register without a verifiable entry, so all
-- legs stay out on the entry gate.
-- npc_twilight_whelp — PORTED 2026-10-16: the EVENT_FADE_ARMOR
-- timer leg is bridgeable (CreateLuaEvent; see whelpEnterCombat /
-- fadeArmorTick below); Reset's RemoveAllAuras leg ported
-- 2026-10-19, the DoZoneInCombat leg has no bridge (documented
-- deviation).
-- achievement_twilight_assist / achievement_twilight_duo /
-- achievement_twilight_zone (target->GetAI()->GetData
-- (TWILIGHT_ACHIEVEMENTS) >= 1 / >= 2 / == 3 — Sartharion's
-- GetData arm, itself documented-only) — no achievement bridge;
-- all three join the unmodeled-achievement queue.

local ENTRY_TENEBRON = 30452
local ENTRY_SHADRON = 30451
local ENTRY_VESPERON = 30449

local SAY_AGGRO  = 0
local SAY_SLAY   = 1
local SAY_DEATH  = 2
local SAY_BREATH = 3

local SPELL_SHADOW_BREATH = 57570
local SPELL_SHADOW_FISSURE = 57579
local SPELL_TWILIGHT_TORMENT_VESP = 57948
local SPELL_GIFT_OF_TWILIGTH_SHA = 57835
local SPELL_POWER_OF_TENEBRON = 61248
local SPELL_POWER_OF_SHADRON = 58105
local SPELL_POWER_OF_VESPERON = 61251
local SPELL_FADE_ARMOR = 60708

local ENTRY_TWILIGHT_WHELP = 30890
local ENTRY_SARTHARION_TWILIGHT_WHELP = 31214

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

-- C++ SelectTarget(Random, 0, 0.0f, true) leg: alive players on the
-- same map/instance; playerOnly=true is idiom-exact; 0.0f = unlimited
-- range approximated by a 500yd cap (the thorim / twilight-corrupter
-- randomPlayerInRange idiom; the old "no target-selection bridge"
-- claim was stale).
local function randomPlayerInRange(creature, minDist, maxDist)
    local mapId = creature:GetMapId()
    local instanceId = creature:GetInstanceId()
    local candidates = {}
    for _, plr in ipairs(GetPlayersInWorld()) do
        if plr:GetMapId() == mapId and plr:GetInstanceId() == instanceId
                and not plr:IsDead() then
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

-- C++ EVENT_SHADOW_FISSURE (dummy_dragonAI::ExecuteEvent,
-- obsidian_sanctum.cpp:387-391): SelectTarget(Random, 0, 0.0f, true)
-- (playerOnly=true, idiom-exact) -> randomPlayerInRange(creature, 0,
-- 500), nil-guarded pick (C++ `if` — skip cast on nil); DoCast 2-arg =
-- non-triggered -> CastSpell(target, spell); re-arm {15s,20s}
-- unconditional (outside the target-if, C++-exact); 5s init (:192).
local function shadowFissureTick(creature, guid)
    local target = randomPlayerInRange(creature, 0, 500)
    if target then
        creature:CastSpell(target, SPELL_SHADOW_FISSURE)
    end
    schedule(guid, "shadowfissure", {15000, 20000}, function()
        shadowFissureTick(creature, guid)
    end)
end

-- C++ EVENT_SHADOW_BREATH (dummy_dragonAI::ExecuteEvent):
-- Talk(SAY_BREATH 3) + DoCastVictim(SPELL_SHADOW_BREATH 57570)
-- (non-triggered), 20s init, re-arm {20s,25s}.
local function shadowBreathTick(creature, guid)
    creature:Talk(SAY_BREATH)
    creature:CastSpell(nil, SPELL_SHADOW_BREATH)
    schedule(guid, "shadowbreath", {20000, 25000}, function()
        shadowBreathTick(creature, guid)
    end)
end

-- C++ npc_vesperon::UpdateAI EVENT_ACOLYTE_VESPERON
-- (obsidian_sanctum.cpp:590-597): instance-gated OpenPortal +
-- DoCastVictim(SPELL_TWILIGHT_TORMENT_VESP 57948), non-triggered,
-- re-arm {60s,70s} — the victim-cast leg only; the DATA_PORTAL_OPEN
-- gate and OpenPortal summon legs have no bridges (documented
-- deviation in header).
local function tormentTick(creature, guid)
    creature:CastSpell(nil, SPELL_TWILIGHT_TORMENT_VESP)
    schedule(guid, "torment", {60000, 70000}, function()
        tormentTick(creature, guid)
    end)
end

-- C++ dummy_dragonAI::JustEngagedWith: Talk(SAY_AGGRO) +
-- DoZoneInCombat + ScheduleEvent(EVENT_SHADOW_FISSURE,
-- EVENT_SHADOW_BREATH) — the Talk and both timer legs are bridgeable
-- (the old "no target-selection bridge" claim for the fissure leg was
-- stale — randomPlayerInRange); the DoZoneInCombat leg has no bridge.
-- Plus the per-drake engage timers: EVENT_HATCH_EGGS
-- (tenebron) and EVENT_ACOLYTE_SHADRON (shadron) payloads are
-- unbridged; EVENT_ACOLYTE_VESPERON (vesperon) ports the
-- victim-cast leg only (tormentTick, 1min init).
local function dragonEnterCombat(event, creature, target)
    creature:Talk(SAY_AGGRO)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "shadowbreath", 20000, function()
        shadowBreathTick(creature, guid)
    end)
    schedule(guid, "shadowfissure", 5000, function()
        shadowFissureTick(creature, guid)
    end)
    if creature:GetEntry() == ENTRY_VESPERON then
        schedule(guid, "torment", 60000, function()
            tormentTick(creature, guid)
        end)
    end
end

local function dragonLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ dummy_dragonAI::KilledUnit: victim->GetTypeId() ==
-- TYPEID_PLAYER gate, then Talk(SAY_SLAY) — the razuvious
-- player-gated variant precedent.
local function dragonTargetDied(event, creature, victim)
    if victim and victim:GetObjectType() == "Player" then
        creature:Talk(SAY_SLAY)
    end
end

local powerAuraByEntry = {
    [ENTRY_TENEBRON] = SPELL_POWER_OF_TENEBRON,
    [ENTRY_SHADRON] = SPELL_POWER_OF_SHADRON,
    [ENTRY_VESPERON] = SPELL_POWER_OF_VESPERON,
}

-- C++ dummy_dragonAI::JustDied: the _canLoot/SetLootRecipient,
-- entry-switched SetBossState(DATA_TENEBRON/SHADRON/VESPERON, DONE),
-- and acolyte KillSelf legs have no loot/instance/kill bridges
-- (documented); Talk(SAY_DEATH) + the entry-switched SELF
-- RemoveAurasDueToSpell(spellId) leg are PORTED — creature:RemoveAura
-- is bridged (stale "only the Talk arm is bridgeable" claim retracted).
-- DoRemoveAurasDueToSpellOnPlayers + Twilight Revenge on Sartharion
-- ride instance/player-list legs (documented). Precision on the
-- player-loop arm (obsidian_sanctum.cpp drake JustDied): the
-- player:RemoveAura / player:HasAura / GetPlayersInWorld pieces ARE
-- bridged, but the arm stays out — the !GetVictim() gate has no
-- player-surface bridge, and the arm's two player self-cast de-phase
-- spells (TWILIGHT_SHIFT_REMOVAL_ALL / TWILIGHT_RESIDUE) have no
-- player:CastSpell bridge, so a removals-only port would strip the
-- aura without the de-phase mechanic and would hit players the C++
-- keeps phased.
local function dragonDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
    creature:Talk(SAY_DEATH)
    local spellId = powerAuraByEntry[creature:GetEntry()]
    if spellId then
        creature:RemoveAura(spellId)
    end
end

local function dragonReset(event, creature)
    cancelTimers(creature:GetGUID())
end

for _, entry in ipairs({ ENTRY_TENEBRON, ENTRY_SHADRON, ENTRY_VESPERON }) do
    RegisterCreatureEvent(entry, 1, dragonEnterCombat)
    RegisterCreatureEvent(entry, 2, dragonLeaveCombat)
    RegisterCreatureEvent(entry, 3, dragonTargetDied)
    RegisterCreatureEvent(entry, 4, dragonDied)
    RegisterCreatureEvent(entry, 23, dragonReset)
end

-- C++ npc_shadron::Reset: the self HasAura-gated strips of
-- SPELL_TWILIGHT_TORMENT_VESP / SPELL_GIFT_OF_TWILIGTH_SHA are PORTED
-- (stale "no aura bridge" claim retracted — HasAura/RemoveAura are
-- bridged); the SetBossState(DATA_PORTAL_OPEN, NOT_STARTED) leg has no
-- instance bridge (documented). Registered after dragonReset so the
-- timer cancel lands first, mirroring the C++ base-then-override order.
local function shadronReset(event, creature)
    if creature:HasAura(SPELL_TWILIGHT_TORMENT_VESP) then
        creature:RemoveAura(SPELL_TWILIGHT_TORMENT_VESP)
    end
    if creature:HasAura(SPELL_GIFT_OF_TWILIGTH_SHA) then
        creature:RemoveAura(SPELL_GIFT_OF_TWILIGTH_SHA)
    end
end
RegisterCreatureEvent(ENTRY_SHADRON, 23, shadronReset)

-- C++ npc_twilight_whelp::Reset (obsidian_sanctum.cpp:983-988):
-- RemoveAllAuras ported 2026-10-19; the DoZoneInCombat leg has no
-- bridge (documented deviation); the bridgeable leg is
-- ScheduleEvent(EVENT_FADE_ARMOR,
-- 1s), seeded here on event 1 per the svalna precedent (Reset-seeded
-- timers re-keyed to combat start). UpdateAI ExecuteEvent:
-- DoCastVictim(SPELL_FADE_ARMOR 60708) (non-triggered), re-arm
-- {5s,10s}, UpdateVictim()-gated.
local function fadeArmorTick(creature, guid)
    creature:CastSpell(nil, SPELL_FADE_ARMOR)
    schedule(guid, "fadearmor", {5000, 10000}, function()
        fadeArmorTick(creature, guid)
    end)
end

local function whelpEnterCombat(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    schedule(guid, "fadearmor", 1000, function()
        fadeArmorTick(creature, guid)
    end)
end

local function whelpLeaveCombat(event, creature)
    cancelTimers(creature:GetGUID())
end

local function whelpDied(event, creature, killer)
    cancelTimers(creature:GetGUID())
end

local function whelpReset(event, creature)
    creature:RemoveAllAuras()
    cancelTimers(creature:GetGUID())
end

for _, entry in ipairs({ ENTRY_TWILIGHT_WHELP, ENTRY_SARTHARION_TWILIGHT_WHELP }) do
    RegisterCreatureEvent(entry, 1, whelpEnterCombat)
    RegisterCreatureEvent(entry, 2, whelpLeaveCombat)
    RegisterCreatureEvent(entry, 4, whelpDied)
    RegisterCreatureEvent(entry, 23, whelpReset)
end

-- New arms (ported 2026-10-18, entries C++-verifiable):
local SPELL_TWILIGHT_SHIFT_ENTER = 57620
local SPELL_TWILIGHT_TORMENT_VESP_ACO = 58853
local SPELL_FLAME_TSUNAMI_DMG_AURA = 57491
local SPELL_FLAME_TSUNAMI = 57494 -- obsidian_sanctum.cpp:78 (visual dummy)
local SPELL_FLAME_TSUNAMI_BUFF = 60430
local NPC_LAVA_BLAZE = 30643
local REACT_PASSIVE = 0
local UNIT_FIELD_FLAGS = 59
local UNIT_FLAG_NON_ATTACKABLE = 0x2
local UNIT_FLAG_NOT_SELECTABLE = 0x2000000
local UNIT_FLAG_INTRO_MASK = UNIT_FLAG_NON_ATTACKABLE + UNIT_FLAG_NOT_SELECTABLE
local ENTRY_ACOLYTE_SHADRON = 31218
local ENTRY_ACOLYTE_VESPERON = 31219
local ENTRY_TWILIGHT_EGG = 30882
local ENTRY_SARTHARION_TWILIGHT_EGG = 31204
local ENTRY_FLAME_TSUNAMI = 30616

-- Shared ev4 cleanup for the acolytes' 28s despawn timers.
local function acolyteDespawnCancel(event, creature)
    cancelTimers(creature:GetGUID())
end

-- C++ npc_acolyte_of_shadron::Reset (obsidian_sanctum.cpp:632-650):
-- me->AddAura(SPELL_TWILIGHT_SHIFT_ENTER, me) — self-aura leg
-- ported; DespawnOrUnsummon(28s) ports as a 28s one-shot
-- self-despawn (arm-if-absent: the C++ despawn countdown keeps
-- running through evade, so the ev23-on-evade refire must not
-- re-arm it; ev4 cancels it); the instance-gated gift-aura legs
-- stay out.
local function acolyteShadronReset(event, creature)
    creature:AddAura(SPELL_TWILIGHT_SHIFT_ENTER)
    local guid = creature:GetGUID()
    local per = timers[guid]
    if not (per and per["despawn"]) then
        schedule(guid, "despawn", 28000, function()
            cancelTimers(guid)
            creature:Despawn()
        end)
    end
end
RegisterCreatureEvent(ENTRY_ACOLYTE_SHADRON, 23, acolyteShadronReset)
RegisterCreatureEvent(ENTRY_ACOLYTE_SHADRON, 4, acolyteDespawnCancel)

-- C++ npc_acolyte_of_vesperon::Reset (obsidian_sanctum.cpp:718-726):
-- me->AddAura(SPELL_TWILIGHT_SHIFT_ENTER, me) + DoCast(me,
-- SPELL_TWILIGHT_TORMENT_VESP_ACO) (2-arg non-triggered self-cast)
-- ported; DespawnOrUnsummon(28s) ports as a 28s one-shot
-- self-despawn (same arm-if-absent/ev4-cancel terms as shadron).
local function acolyteVesperonReset(event, creature)
    creature:AddAura(SPELL_TWILIGHT_SHIFT_ENTER)
    creature:CastSpell(creature, SPELL_TWILIGHT_TORMENT_VESP_ACO)
    local guid = creature:GetGUID()
    local per = timers[guid]
    if not (per and per["despawn"]) then
        schedule(guid, "despawn", 28000, function()
            cancelTimers(guid)
            creature:Despawn()
        end)
    end
end
RegisterCreatureEvent(ENTRY_ACOLYTE_VESPERON, 23, acolyteVesperonReset)

-- C++ npc_acolyte_of_vesperon::JustDied (obsidian_sanctum.cpp:728):
-- me->RemoveAurasDueToSpell(SPELL_TWILIGHT_TORMENT_VESP_ACO) —
-- self leg ported; the instance/vesperon/player legs stay out.
local function acolyteVesperonDied(event, creature, killer)
    creature:RemoveAura(SPELL_TWILIGHT_TORMENT_VESP_ACO)
    cancelTimers(creature:GetGUID())
end
RegisterCreatureEvent(ENTRY_ACOLYTE_VESPERON, 4, acolyteVesperonDied)

-- C++ npc_twilight_eggs::Reset (obsidian_sanctum.cpp:804-809):
-- me->AddAura(SPELL_TWILIGHT_SHIFT_ENTER, me) ported; the
-- EVENT_TWILIGHT_EGGS 20s payload (SetBossState + SpawnWhelps)
-- has no summon / instance bridges.
local function twilightEggReset(event, creature)
    creature:AddAura(SPELL_TWILIGHT_SHIFT_ENTER)
end
for _, entry in ipairs({ ENTRY_TWILIGHT_EGG, ENTRY_SARTHARION_TWILIGHT_EGG }) do
    RegisterCreatureEvent(entry, 23, twilightEggReset)
end

-- C++ npc_flame_tsunami (obsidian_sanctum.cpp:866-916): Reset —
-- SetReactState(REACT_PASSIVE) + SetFlag(UNIT_FIELD_FLAGS,
-- NON_ATTACKABLE | NOT_SELECTABLE) ported (both bridged);
-- EVENT_TSUNAMI_TIMER DoCast(me, SPELL_FLAME_TSUNAMI_DMG_AURA)
-- (2-arg non-triggered self-cast), 100ms init / 500ms re-arm —
-- ported via CreateLuaEvent, seeded on Reset, cancelled on
-- OnRemove (event 37); EVENT_TSUNAMI_BUFF (1s init / 1s re-arm) —
-- the nearest live lava blaze within 10yd self-casts
-- SPELL_FLAME_TSUNAMI_BUFF (triggered) via the FindNearestCreature
-- bridge — ported. The ctor legs (SetDisplayId 11686 + AddAura
-- SPELL_FLAME_TSUNAMI 57494) are ported via tsunamiSpawn (event 5)
-- below — the stale "no spawn hook / display bridge" claim
-- retracted.
local function tsunamiTick(creature, guid)
    creature:CastSpell(creature, SPELL_FLAME_TSUNAMI_DMG_AURA)
    schedule(guid, "tsunami", 500, function()
        tsunamiTick(creature, guid)
    end)
end

-- C++ EVENT_TSUNAMI_BUFF (obsidian_sanctum.cpp:894-897).
local function tsunamiBuffTick(creature, guid)
    local blaze = creature:FindNearestCreature(NPC_LAVA_BLAZE, 10)
    if blaze then
        blaze:CastSpell(blaze, SPELL_FLAME_TSUNAMI_BUFF, true)
    end
    schedule(guid, "tsunamiBuff", 1000, function()
        tsunamiBuffTick(creature, guid)
    end)
end

local function tsunamiReset(event, creature)
    local guid = creature:GetGUID()
    cancelTimers(guid)
    creature:SetReactState(REACT_PASSIVE)
    schedule(guid, "tsunami", 100, function()
        tsunamiTick(creature, guid)
    end)
    schedule(guid, "tsunamiBuff", 1000, function()
        tsunamiBuffTick(creature, guid)
    end)
    creature:SetFlag(UNIT_FIELD_FLAGS, UNIT_FLAG_INTRO_MASK)
end

local function tsunamiRemove(event, creature)
    cancelTimers(creature:GetGUID())
end
RegisterCreatureEvent(ENTRY_FLAME_TSUNAMI, 23, tsunamiReset)
RegisterCreatureEvent(ENTRY_FLAME_TSUNAMI, 37, tsunamiRemove)

-- C++ npc_flame_tsunamiAI ctor (obsidian_sanctum.cpp:868-872):
-- SetDisplayId(11686) + AddAura(SPELL_FLAME_TSUNAMI 57494, me).
-- The stale "no spawn / display bridges" claim is retracted —
-- creature:SetDisplayId (lua_creature_events.go:643) and
-- creature:AddAura (self-only, lua_creature_events.go:249) are
-- bridged; event 5 (OnSpawn) is the ctor-equivalent hook
-- (the felmyst YELL_BIRTH precedent).
local function tsunamiSpawn(event, creature)
    creature:SetDisplayId(11686)
    creature:AddAura(SPELL_FLAME_TSUNAMI)
end
RegisterCreatureEvent(ENTRY_FLAME_TSUNAMI, 5, tsunamiSpawn)
