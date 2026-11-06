package core

import (
	"errors"
	"time"
)

// Command — намерение (игрока, ведущего или таймера). Game.Handle либо
// отклоняет его с ошибкой и ничего не меняет, либо порождает события.
type Command interface{ isCommand() }

// StartRun — старт игры: маршруты, расписание стартов, старт команд.
type StartRun struct{}

// StartTeam — ручной старт команды (start.mode: manual или досрочно).
type StartTeam struct {
	TeamID string
}

// SubmitAnswer — попытка ответа.
//
//   - KindCode, KindGeo: LevelID можно не указывать — тогда ответ ищется
//     по всем открытым уровням команды (удобно для storm/graph).
//   - KindPhoto, KindText: нужен LevelID и цель — BonusID или AnswerIdx;
//     без цели берётся первый подходящий непроверенный ответ уровня.
type SubmitAnswer struct {
	TeamID        string
	PlayerID      string
	ClientEventID string // идемпотентность офлайн-очереди

	LevelID   string
	BonusID   string
	AnswerIdx *int

	Kind     AnswerKind
	Value    string
	Lat      float64
	Lng      float64
	Accuracy float64 // метры
	MediaKey string
}

// RequestHint — открыть подсказку по запросу (со штрафом).
type RequestHint struct {
	TeamID    string
	PlayerID  string
	IsCaptain bool // иначе капитанство проверяется по PlayerID
	LevelID   string
	HintIdx   int
}

// Moderate — решение по ответу на модерации.
type Moderate struct {
	Ref      int64 // Seq события answer_submitted
	Approved bool
	Comment  string
	By       string
}

// Override — ручное действие ведущего с командой.
type Override struct {
	TeamID  string
	Action  OverrideAction
	LevelID string
	HintIdx int
	Time    time.Duration // adjust_time: + штраф, − компенсация
	Points  int           // adjust_points
	Reason  string        // обязательно
	By      string
}

// SetLeaderboard — показать (true) / скрыть (false) / вернуть по сценарию (nil).
type SetLeaderboard struct {
	Visible *bool
}

type SendBroadcast struct {
	Text string
}

// NoteScenarioChanged — записать в журнал, что сценарий заменён.
// Саму замену делает runner.Manager; событие нужно для истории
// и чтобы клиенты получили уведомление и перечитали состояние.
type NoteScenarioChanged struct {
	VersionID string
	Slug      string
	Version   int
}

// ApproveResults — утвердить итоги (только после завершения игры).
// Можно повторить после правок — итоги перезапишутся.
type ApproveResults struct {
	Message string // общее сообщение командам, например «награждение в 18:00 у фонтана»
}

// FinishRun — завершить игру для всех.
type FinishRun struct{}

// Tick — проверка таймеров: старты по расписанию, автоподсказки,
// лимит времени, заморозка лидерборда.
type Tick struct{}

func (RegisterTeam) isCommand()        {}
func (StartRun) isCommand()            {}
func (StartTeam) isCommand()           {}
func (SubmitAnswer) isCommand()        {}
func (RequestHint) isCommand()         {}
func (Moderate) isCommand()            {}
func (Override) isCommand()            {}
func (SetLeaderboard) isCommand()      {}
func (SendBroadcast) isCommand()       {}
func (FinishRun) isCommand()           {}
func (NoteScenarioChanged) isCommand() {}
func (ApproveResults) isCommand()      {}
func (Tick) isCommand()                {}

var (
	ErrBadCommand         = errors.New("некорректная команда")
	ErrRunStarted         = errors.New("игра уже началась")
	ErrRunNotRunning      = errors.New("игра не идёт")
	ErrNoTeams            = errors.New("нет зарегистрированных команд")
	ErrTeamExists         = errors.New("команда с таким id уже есть")
	ErrUnknownTeam        = errors.New("команда не найдена")
	ErrTeamNotPlaying     = errors.New("команда сейчас не играет")
	ErrTeamAlreadyStarted = errors.New("команда уже стартовала")
	ErrUnknownLevel       = errors.New("уровень не найден")
	ErrLevelNotOpen       = errors.New("уровень не открыт")
	ErrEmptyAnswer        = errors.New("пустой ответ")
	ErrRateLimited        = errors.New("слишком много неверных ответов, подождите")
	ErrNoTarget           = errors.New("не найдено задание для этого ответа")
	ErrAlreadyPending     = errors.New("ответ уже на проверке")
	ErrUnknownHint        = errors.New("подсказка не найдена")
	ErrHintAlreadyOpen    = errors.New("подсказка уже открыта")
	ErrNotAllowed         = errors.New("действие недоступно")
	ErrNotPending         = errors.New("ответ не ждёт модерации")
	ErrReasonRequired     = errors.New("укажите причину")
	ErrRunNotFinished     = errors.New("игра ещё не завершена")
)

// Outcome — результат обработки команды.
type Outcome struct {
	Events []Event
	Answer *AnswerResult // для SubmitAnswer
}
