-- Pyroguard Emberseer (Blackrock Spire; Hall of Blackhand) --
-- Lua port of src/server/scripts/EasternKingdoms/BlackrockMountain/
-- BlackrockSpire/boss_pyroguard_emberseer.cpp
-- (boss_pyroguard_emberseer — BossAI combat scheduler; the file's
-- second class, npc_blackhand_incarcerator, is ported separately in
-- npc_blackhand_incarcerator.lua). Boss-script unit per
-- eastern_kingdoms_script_loader.cpp order (AddSC_boss_pyroguardemberseer
-- follows AddSC_boss_quatermasterzigris in the Blackrock Spire block).
-- Entry (verifiable from the C++ sources): blackrock_spire.h names
-- NPC_PYROGAURD_EMBERSEER = 9816 in the BRS creatures enum (BRSDataTypes
-- DATA_PYROGAURD_EMBERSEER = 9). The creature_template ScriptName
-- binding is DB-side (no TDB in this workspace), but the entry itself
-- is C++-verifiable so the port is registered (quartermaster_zigris /
-- boss_the_beast precedent). GetBlackrockSpireAI is a GetInstanceAI
-- retrieval wrapper only (blackrock_spire.h:129-132).
-- Eluna creature events: 1 OnEnterCombat, 2 OnLeaveCombat, 4 OnDied,
-- 23 OnReset. Driven by CreateLuaEvent timers; melee is engine-driven
-- in Go (creature combat tick), like C++ DoMeleeAttackIfReady.
-- Ported arms (C++ JustEngagedWith event schedule + UpdateAI combat
-- event machine):
-- EVENT_FIRE_SHIELD: self-cast FIRE_SHIELD 13376, 3s init -> 3s loop
-- (DoCast(me) path — creature:CastSpell(creature, spell),
-- twilight_corrupter convention).
-- EVENT_FIRENOVA: self-cast FIRENOVA 23462, 6s init -> 6s loop.
-- EVENT_FLAMEBUFFET: self-cast FLAMEBUFFET 23341, 3s init -> 14s loop.
-- EVENT_PYROBLAST: random-player PYROBLAST 17274 — SelectTarget(Random,
-- 0, 100, true) is playerOnly, ported via the threat list
-- (GetAITargets: players alive within 100yd); 14s init -> 15s
-- loop, re-arm unconditional (C++ Repeat sits outside the target
-- check), non-triggered DoCast(target).
-- CASTING gates ported (combat branch only): C++ UpdateAI pre-loop
-- gate (:250) + in-loop gate (:278) are a 500ms IsCasting defer under
-- each arm's own key (live, lua_creature_events.go:1950; mal_ganis
-- idiom) -- a gated arm fires nothing and consumes no re-arm. The
-- out-of-combat branch has no gate; its fireshield ticks stay
-- unmodeled (port starts fireshield on combat entry). The C++
-- scheduler is driven from JustEngagedWith and drained by UpdateAI;
-- the port schedules from OnEnterCombat(1) and cancels on 2/4/23
-- (maiden convention). C++ schedules EVENT_FIRE_SHIELD at Reset and
-- ticks it out of combat as well; the port starts it on
-- OnEnterCombat(1) — the out-of-combat ticks are unmodeled.
-- Unmodeled (documented-only): the whole pre-fight event is gated on
-- instance / SetData bridges that do not exist on the Lua surface.
-- The SpellHit arms ARE ported below (ev14 onSpellHit, DORMANT —
-- hook + handler fully bridged, producers unported; see the handler
-- comment). The 2026-10-24 stay-out grounds are stale on both legs:
-- Go's cast path DOES fan out entry-area implicit targets
-- (boss_ai.go:2452 isEntryAreaSpell leg), so the incarcerator's
-- self-cast 15281 may resolve onto emberseer via its DBC implicit
-- target (no Spell.dbc in this workspace to confirm the 15281
-- record); and the periodic-trigger engine IS live (spells.go:44960
-- case 23 -> castPeriodicTriggerFromCreature, a full triggered
-- cast), so 16048's ticks would fire 16049 once 16048 is applied —
-- but 16048 itself is applied only by the unported EVENT_PRE_FIGHT_2
-- machine (instance ProcessEvent -> SetData, unbridged).
-- Reset's RemoveAura(16047/16048/16049) rides the SpellHit leg-1
-- Reset twin below; EVENT_RESPAWN 5s (instance->SetData
-- (DATA_BLACKHAND_INCARCERATOR, 1) + SetBossState(NOT_STARTED));
-- (Reset's UNIT_FLAG_NOT_SELECTABLE + SetImmuneToPC(true) pair IS
-- ported — creature:SetFlag(59, 0x2000000) / SetFlag(59, 0x100),
-- boss_pyroguard_emberseer.cpp:85-86.)
-- SetData(type, 1) -> EVENT_PLAYER_CHECK 5s (player-list aura scan
-- for SPELL_EMBERSEER_OBJECT_VISUAL 16532, instance->SetBossState
-- (IN_PROGRESS)); EVENT_PRE_FIGHT_1 (GetCreatureListWithEntryInGrid
-- NPC_BLACKHAND_INCARCERATOR 10316, 35.0f -> SetImmuneToAll(false) +
-- InterruptSpell + DoZoneInCombat, RemoveAura(15282),
-- EVENT_PRE_FIGHT_2 at 32s) — re-triaged 2026-11-05: every leg is
-- bridged now (GetCreaturesInGrid, InterruptSpell(2), DoZoneInCombat,
-- RemoveAura, the SetImmuneToAll flag pair) but the arm stays out —
-- its only scheduler is the instance ProcessEvent
-- (EVENT_PYROGUARD_EMBERSEER) -> Emberseer->AI()->SetData(1, 1)
-- (instance_blackrock_spire.cpp:274-281), and neither instance
-- ProcessEvent forwarding nor cross-AI SetData dispatch is bridged
-- (creature:SetData is a per-motion store, tick-222 precedent);
-- EVENT_PRE_FIGHT_2 (self-cast 16245 +
-- 16048, Talk(EMOTE_ONE_STACK)); EVENT_ENTER_COMBAT (AttackStart(
-- SelectNearestPlayer(30.0f)) — the 2s ENTER_COMBAT leg IS ported
-- inside the SpellHit 16049 20-stack arm below). The SpellHit arms
-- (15281 -> self-cast 15282 + Reset if no aura; 16049 stack latches:
-- EMOTE_TEN_STACK at 10, at 20 remove 16245, cast 16047, Talk
-- EMOTE_FREE_OF_BONDS / YELL_FREE_OF_BONDS, clear flags,
-- EVENT_ENTER_COMBAT at 2s) are PORTED below (ev14 onSpellHit,
-- DORMANT — see above).
-- (instance / SetData / SelectTarget precedents: instance / the_beast /
-- draenei_survivor).
-- Reset's _Reset() and JustDied's _JustDied() are NOT called in C++
-- (Reset :83-99 schedules EVENT_RESPAWN instead; JustDied :118-124
-- writes the boss state directly) — the cancel/re-arm on combat events
-- is the _Reset/_JustDied bookkeeping equivalent. JustDied's
-- UpdateRunes(GO_STATE_READY) is PORTED below via
-- creature:FindNearestGameObject (7 distinct rune entries; C++ used
-- instance GetGuidData), followed by the direct SetBossState(DONE)
-- write (DATA_PYROGAURD_EMBERSEER = 9; live — instance
-- SetBossNumber(23), doors + OnGameObjectCreate read state 9; gyth
-- convention). No ev1 IN_PROGRESS leg: C++ JustEngagedWith never calls
-- the BossAI base — IN_PROGRESS is written only by the unported
-- EVENT_PLAYER_CHECK machine. The ev23 NOT_STARTED write is
-- EVENT_RESPAWN (5s timer, unconditional — no _Reset != DONE gate).

local SPELL_FIRE_SHIELD = 13376
local SPELL_FIRENOVA = 23462
local SPELL_FLAMEBUFFET = 23341
local SPELL_PYROBLAST = 17274
local SPELL_ENCAGE_EMBERSEER = 15281
local SPELL_ENCAGED_EMBERSEER = 15282
local SPELL_FREEZE_ANIM = 16245
local SPELL_EMBERSEER_GROWING = 16048
local SPELL_EMBERSEER_GROWING_TRIGGER = 16049
local SPELL_EMBERSEER_FULL_STRENGTH = 16047

local EMOTE_TEN_STACK = 1
local EMOTE_FREE_OF_BONDS = 2
local YELL_FREE_OF_BONDS = 3

local ENTRY_PYROGAURD_EMBERSEER = 9816
local DATA_PYROGAURD_EMBERSEER = 9 -- blackrock_spire.h:39
local EMBERSEER_RUNES = {175266, 175267, 175268, 175269, 175270, 175271, 175272} -- blackrock_spire.h:115-121

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

-- C++ boss_pyroguard_emberseerAI::Reset
-- (boss_pyroguard_emberseer.cpp:83-99) as invoked from the SpellHit
-- 15281 leg: flags first (NOT_SELECTABLE + SetImmuneToPC(true), the
-- SetFlag(59, 0x100) twin already used by onCombatEnd), timers
-- cancelled (events.Reset), the growing/full-strength auras removed,
-- and EVENT_RESPAWN re-scheduled 5s out (its SetBossState(NOT_STARTED)
-- write; the companion instance->SetData(DATA_BLACKHAND_INCARCERATOR,
-- 1) leg stays out — no instance SetData bridge). The Reset
-- EVENT_FIRE_SHIELD 3s re-arm stays unmodeled with the other
-- out-of-combat fireshield ticks (see header).
local function resetEmberseer(creature, guid)
    creature:SetFlag(59, 0x2000000)
    creature:SetFlag(59, 0x100)
    cancelTimers(guid)
    creature:RemoveAura(SPELL_EMBERSEER_FULL_STRENGTH)
    creature:RemoveAura(SPELL_EMBERSEER_GROWING)
    creature:RemoveAura(SPELL_EMBERSEER_GROWING_TRIGGER)
    schedule(guid, "respawn", 5000, function()
        creature:SetBossState(DATA_PYROGAURD_EMBERSEER, 0) -- NOT_STARTED
    end)
end

local function onFireShield(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "fireshield", 500, function() onFireShield(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FIRE_SHIELD)
    schedule(guid, "fireshield", 3000, function() onFireShield(creature, guid) end)
end

local function onFireNova(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "firenova", 500, function() onFireNova(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FIRENOVA)
    schedule(guid, "firenova", 6000, function() onFireNova(creature, guid) end)
end

local function onFlameBuffet(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "flamebuffet", 500, function() onFlameBuffet(creature, guid) end)
        return
    end
    creature:CastSpell(creature, SPELL_FLAMEBUFFET)
    schedule(guid, "flamebuffet", 14000, function() onFlameBuffet(creature, guid) end)
end

-- C++ SelectTarget(Random, 0, dist, true): random threat-list
-- player alive within range (UnitAI.h:162-176 — the old
-- instance-player pool could pick players who never engaged).
local function randomThreatPlayerInRange(creature, range)
    local candidates = {}
    for _, plr in ipairs(creature:GetAITargets()) do
        if plr:GetObjectType() == "Player" and not plr:IsDead()
                and creature:GetDistance(plr) - creature:GetCombatReach() - 1.5 < range then -- IsWithinCombatRange twin (Unit.cpp:583; player reach 1.5)
            candidates[#candidates + 1] = plr
        end
    end
    if #candidates == 0 then
        return nil
    end
    return candidates[math.random(#candidates)]
end

-- C++ EVENT_PYROBLAST: SelectTarget(Random, 0, 100, true) playerOnly,
-- DoCast(target, 17274) non-triggered, 14s init, Repeat(15s) outside
-- the target check (re-arm unconditional).
local function onPyroblast(creature, guid)
    if creature:IsCasting() then
        schedule(guid, "pyroblast", 500, function() onPyroblast(creature, guid) end)
        return
    end
    local target = randomThreatPlayerInRange(creature, 100)
    if target then
        creature:CastSpell(target, SPELL_PYROBLAST)
    end
    schedule(guid, "pyroblast", 15000, function() onPyroblast(creature, guid) end)
end

local function onEnterCombat(_, creature)
    -- C++ boss_pyroguard_emberseerAI::JustEngagedWith (boss_pyroguard_emberseer.cpp:110-115)
    -- arms only the three event schedules — no zone-combat arm. The
    -- incarcerator grid-loop DoZoneInCombat leg is npc_blackhand_incarceratorAI::JustEngagedWith
    -- (boss_pyroguard_emberseer.cpp:318-328), ported in npc_blackhand_incarcerator.lua
    -- (bridge live 2026-10-23); 817bd0cc had misattributed it to this handler — moved.
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    schedule(guid, "fireshield", 3000, function() onFireShield(creature, guid) end)
    schedule(guid, "firenova", 6000, function() onFireNova(creature, guid) end)
    schedule(guid, "flamebuffet", 3000, function() onFlameBuffet(creature, guid) end)
    schedule(guid, "pyroblast", 14000, function() onPyroblast(creature, guid) end)
end

local function onCombatEnd(event, creature)
    -- C++ Reset (boss_pyroguard_emberseer.cpp:83-86): NOT_SELECTABLE +
    -- SetImmuneToPC(true) [SetFlag(59, 0x100)] — the flag pair comes
    -- first in arm order, before events.Reset and the aura cleanup.
    creature:SetFlag(59, 0x2000000)
    creature:SetFlag(59, 0x100)
    local guid = creature:GetGUIDLow()
    cancelTimers(guid)
    if event == 4 then
        -- C++ JustDied (boss_pyroguard_emberseer.cpp:118-124):
        -- UpdateRunes(GO_STATE_READY) first, then the direct
        -- instance->SetBossState(DATA_PYROGAURD_EMBERSEER, DONE) write
        -- (illidan event-4 precedent for the rune leg).
        for _, entry in ipairs(EMBERSEER_RUNES) do
            local rune = creature:FindNearestGameObject(entry, 100.0)
            if rune then rune:SetGoState(1) end -- GO_STATE_READY
        end
        creature:SetBossState(DATA_PYROGAURD_EMBERSEER, 3) -- DONE
    elseif event == 23 and creature:IsAlive() then
        -- C++ EVENT_RESPAWN (boss_pyroguard_emberseer.cpp:183-188):
        -- Reset schedules it 5s out; it writes SetBossState(NOT_STARTED)
        -- unconditionally (no _Reset != DONE gate — Reset never calls
        -- _Reset). The companion SetData(DATA_BLACKHAND_INCARCERATOR, 1)
        -- leg stays out (no instance SetData bridge). Alive-gated: Go
        -- fires ev23 on the death path too, and C++ JustDied owns DONE.
        -- Cancelled on re-engage by onEnterCombat's cancelTimers,
        -- matching C++ swallowing the event in the combat branch.
        schedule(guid, "respawn", 5000, function()
            creature:SetBossState(DATA_PYROGAURD_EMBERSEER, 0) -- NOT_STARTED
        end)
    end
end

-- PORTED (DORMANT) — C++ boss_pyroguard_emberseerAI::SpellHit
-- (boss_pyroguard_emberseer.cpp:127-156), event 14
-- (boss_ai.go:4231-4243: CREATURE_EVENT_ON_HIT_BY_SPELL, args
-- (event, creature, caster, spellid); magtheridonSpellHit precedent).
-- DORMANT: the hook + handler are fully bridged, but neither
-- producer fires on a live path yet — leg 1 needs the incarcerator's
-- self-cast 15281 to resolve onto emberseer via its DBC implicit
-- target (entry-area fan-out exists, boss_ai.go:2452; no Spell.dbc
-- in this workspace to confirm the 15281 record), and leg 2 needs
-- 16048 applied by the unported EVENT_PRE_FIGHT_2 machine (its
-- periodic trigger itself IS live, spells.go:44960 case 23).
-- Count reads: no GetAuraCount bridge, but GetStackAmount(spell) IS
-- the count twin (lua_creature_events.go:1804; GetAuraCount sums
-- stack amounts, Unit.cpp:4644-4656 — single-aura case; spell_holiday
-- precedent). Ordering is C++-exact as a literal ==10 / ==20 read:
-- C++ applies the hit's aura effects in the per-effect
-- DoTargetSpellHit loop BEFORE DoDamageAndTriggers fires AI SpellHit
-- (Spell.cpp:3555-3566, :2629), and Go likewise applies auras
-- (creatureEffectApplyAura, boss_ai.go:3793) before the OnHit
-- fireSpellHit (boss_ai.go:3866) — the read sees the post-hit stack.
-- (The npc_imprisoned_beryl_sorcerer header claim that SpellHit fires
-- before aura apply is wrong for this hook: its Spell.cpp:2629 cite
-- IS the post-effects DoDamageAndTriggers arm.)
-- Leg 1 (:129-136): 15281 with no 15282 aura -> self-cast 15282 +
-- Reset(). Leg 2 (:138-155): 16049 stacks — 10: Talk(EMOTE_TEN_STACK
-- = 1); 20: RemoveAura(16245), self-cast 16047, Talk(EMOTE_FREE_OF_
-- BONDS = 2) + Talk(YELL_FREE_OF_BONDS = 3), clear NOT_SELECTABLE +
-- the SetImmuneToPC(false) leg (no direct bridge — RemoveFlag(59,
-- 0x100), the inverse of this file's SetFlag(59, 0x100) =
-- SetImmuneToPC(true) twin), EVENT_ENTER_COMBAT 2s -> AttackStart(
-- SelectNearestPlayer(30.0f)) = GetNearestPlayer(30.0), alive-only
-- by default (lua_worldobject_range.go:21-25).
local function onSpellHit(_, creature, _, spellId)
    if spellId == SPELL_ENCAGE_EMBERSEER then
        if creature:GetStackAmount(SPELL_ENCAGED_EMBERSEER) == 0 then
            creature:CastSpell(creature, SPELL_ENCAGED_EMBERSEER)
            resetEmberseer(creature, creature:GetGUIDLow())
        end
        return
    end
    if spellId == SPELL_EMBERSEER_GROWING_TRIGGER then
        local stacks = creature:GetStackAmount(SPELL_EMBERSEER_GROWING_TRIGGER)
        if stacks == 10 then
            creature:Talk(EMOTE_TEN_STACK)
        end
        if stacks == 20 then
            creature:RemoveAura(SPELL_FREEZE_ANIM)
            creature:CastSpell(creature, SPELL_EMBERSEER_FULL_STRENGTH)
            creature:Talk(EMOTE_FREE_OF_BONDS)
            creature:Talk(YELL_FREE_OF_BONDS)
            creature:RemoveFlag(59, 0x2000000)
            creature:RemoveFlag(59, 0x100)
            local guid = creature:GetGUIDLow()
            schedule(guid, "entercombat", 2000, function()
                local target = creature:GetNearestPlayer(30.0)
                if target then
                    creature:AttackStart(target)
                end
            end)
        end
    end
end

RegisterCreatureEvent(ENTRY_PYROGAURD_EMBERSEER, 1, onEnterCombat)
RegisterCreatureEvent(ENTRY_PYROGAURD_EMBERSEER, 2, onCombatEnd)
RegisterCreatureEvent(ENTRY_PYROGAURD_EMBERSEER, 4, onCombatEnd)
RegisterCreatureEvent(ENTRY_PYROGAURD_EMBERSEER, 14, onSpellHit)
RegisterCreatureEvent(ENTRY_PYROGAURD_EMBERSEER, 23, onCombatEnd)
