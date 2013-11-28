package monochrome

import (
	"github.com/hajimehoshi/go-ebiten"
	"github.com/hajimehoshi/go-ebiten/graphics"
	"github.com/hajimehoshi/go-ebiten/graphics/matrix"
	"image"
	_ "image/png"
	"os"
)

const (
	ebitenTextureWidth  = 57
	ebitenTextureHeight = 26
)

type Monochrome struct {
	ebitenTextureId graphics.TextureId
	ch              chan bool
	colorMatrix     matrix.Color
	geometryMatrix  matrix.Geometry
}

func New() *Monochrome {
	return &Monochrome{
		ch:             make(chan bool),
		colorMatrix:    matrix.IdentityColor(),
		geometryMatrix: matrix.IdentityGeometry(),
	}
}

func (game *Monochrome) InitTextures(tf graphics.TextureFactory) {
	file, err := os.Open("images/ebiten.png")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		panic(err)
	}
	if game.ebitenTextureId, err = tf.CreateTextureFromImage(img); err != nil {
		panic(err)
	}

	go game.update()
}

func mean(a, b matrix.Color, k float64) matrix.Color {
	dim := a.Dim()
	result := matrix.Color{}
	for i := 0; i < dim-1; i++ {
		for j := 0; j < dim; j++ {
			result.Elements[i][j] =
				a.Elements[i][j]*(1-k) +
					b.Elements[i][j]*k
		}
	}
	return result
}

func (game *Monochrome) update() {
	const fps = 60
	colorI := matrix.IdentityColor()
	colorMonochrome := matrix.Monochrome()
	for {
		for i := 0; i < fps; i++ {
			<-game.ch
			rate := float64(i) / float64(fps)
			game.colorMatrix = mean(colorI, colorMonochrome, rate)
			game.ch <- true
		}
		for i := 0; i < fps; i++ {
			<-game.ch
			game.colorMatrix = colorMonochrome
			game.ch <- true
		}
		for i := 0; i < fps; i++ {
			<-game.ch
			rate := float64(i) / float64(fps)
			game.colorMatrix = mean(colorMonochrome, colorI, rate)
			game.ch <- true
		}
		for i := 0; i < fps; i++ {
			<-game.ch
			game.colorMatrix = colorI
			game.ch <- true
		}
	}
}

func (game *Monochrome) Update(context ebiten.GameContext) {
	game.ch <- true
	<-game.ch

	game.geometryMatrix = matrix.IdentityGeometry()
	tx := context.ScreenWidth()/2 - ebitenTextureWidth/2
	ty := context.ScreenHeight()/2 - ebitenTextureHeight/2
	game.geometryMatrix.Translate(float64(tx), float64(ty))
}

func (game *Monochrome) Draw(g graphics.Context) {
	g.Fill(128, 128, 255)

	g.DrawTexture(game.ebitenTextureId,
		game.geometryMatrix, game.colorMatrix)
}
